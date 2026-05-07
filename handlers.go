package main

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const (
	maxBodySize       = 1 << 20 // 1 MB
	maxServiceNameLen = 255
)

func subscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		createSubscription(w, r)
		return
	}

	if r.Method == http.MethodGet {
		listSubscriptions(w, r)
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func oneSubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/subscriptions/")
	if id == "" || strings.Contains(id, "/") {
		sendError(w, http.StatusNotFound, "Not found")
		return
	}

	if r.Method == http.MethodGet {
		getSubscription(w, id)
		return
	}

	if r.Method == http.MethodPut {
		updateSubscription(w, r, id)
		return
	}

	if r.Method == http.MethodDelete {
		deleteSubscription(w, id)
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func createSubscription(w http.ResponseWriter, r *http.Request) {
	logDebug("createSubscription: reading request body")

	var req CreateRequest
	if !readJSONBody(w, r, &req) {
		return
	}

	logDebug("createSubscription: validating fields service_name=%q price=%d user_id=%q start_date=%q end_date=%q allow_behindhand_date=%v",
		req.ServiceName, req.Price, req.UserID, req.StartDate, req.EndDate, req.AllowBehindhandDate)

	cleanName, err := cleanServiceName(req.ServiceName)
	if err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.ServiceName = cleanName

	if req.Price <= 0 {
		sendError(w, http.StatusBadRequest, "Field 'price' must be greater than 0")
		return
	}

	if !isUUID(req.UserID) {
		sendError(w, http.StatusBadRequest, "Field 'user_id' must be a valid UUID")
		return
	}

	if req.StartDate == "" {
		sendError(w, http.StatusBadRequest, "Field 'start_date' is required")
		return
	}

	start, err := parseDate(req.StartDate)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Field 'start_date' "+err.Error())
		return
	}

	if err := checkStartDateNotPast(start, req.AllowBehindhandDate); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	var endParam interface{}
	if req.EndDate != "" {
		end, err := parseDate(req.EndDate)
		if err != nil {
			sendError(w, http.StatusBadRequest, "Field 'end_date' "+err.Error())
			return
		}
		if end.Before(start) {
			sendError(w, http.StatusBadRequest, "Field 'end_date' must not be earlier than 'start_date'")
			return
		}
		endParam = end
	}

	id := makeNewID()
	logDebug("createSubscription: generated id=%s, inserting into database", id)

	var sub Subscription
	var dbStart time.Time
	var dbEnd sql.NullTime

	err = db.QueryRow(`
		INSERT INTO subscriptions (id, service_name, price, user_id, start_date, end_date)
		VALUES ($1::uuid, $2, $3, $4::uuid, $5, $6)
		RETURNING id::text, service_name, price, user_id::text, start_date, end_date, created_at, updated_at`,
		id, req.ServiceName, req.Price, req.UserID, start, endParam,
	).Scan(&sub.ID, &sub.ServiceName, &sub.Price, &sub.UserID,
		&dbStart, &dbEnd, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		logError("Failed to create subscription: %v", err)
		sendError(w, http.StatusInternalServerError, "Failed to create subscription")
		return
	}

	sub.StartDate = formatDate(dbStart)
	if dbEnd.Valid {
		sub.EndDate = formatDate(dbEnd.Time)
	}

	logInfo("Created subscription %s (service=%q, user=%s)", sub.ID, sub.ServiceName, sub.UserID)
	sendJSON(w, http.StatusCreated, sub)
}

func getSubscription(w http.ResponseWriter, id string) {
	logDebug("getSubscription: id=%s", id)

	if !isUUID(id) {
		sendError(w, http.StatusBadRequest, "Subscription ID must be a valid UUID")
		return
	}

	var sub Subscription
	var dbStart time.Time
	var dbEnd sql.NullTime

	err := db.QueryRow(`
		SELECT id::text, service_name, price, user_id::text, start_date, end_date, created_at, updated_at
		FROM subscriptions
		WHERE id = $1::uuid`,
		id,
	).Scan(&sub.ID, &sub.ServiceName, &sub.Price, &sub.UserID,
		&dbStart, &dbEnd, &sub.CreatedAt, &sub.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		logDebug("getSubscription: id=%s not found", id)
		sendError(w, http.StatusNotFound, "Subscription not found")
		return
	}
	if err != nil {
		logError("Failed to retrieve subscription %s: %v", id, err)
		sendError(w, http.StatusInternalServerError, "Failed to retrieve subscription")
		return
	}

	sub.StartDate = formatDate(dbStart)
	if dbEnd.Valid {
		sub.EndDate = formatDate(dbEnd.Time)
	}

	logDebug("getSubscription: returning record for id=%s", id)
	sendJSON(w, http.StatusOK, sub)
}

func listSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	serviceName := r.URL.Query().Get("service_name")

	logDebug("listSubscriptions: filters user_id=%q service_name=%q", userID, serviceName)

	if userID != "" && !isUUID(userID) {
		sendError(w, http.StatusBadRequest, "Query parameter 'user_id' must be a valid UUID")
		return
	}

	rows, err := db.Query(`
		SELECT id::text, service_name, price, user_id::text, start_date, end_date, created_at, updated_at
		FROM subscriptions
		WHERE ($1 = '' OR user_id::text = $1)
		  AND ($2 = '' OR service_name = $2)
		ORDER BY created_at DESC`,
		userID, serviceName)
	if err != nil {
		logError("Failed to list subscriptions: %v", err)
		sendError(w, http.StatusInternalServerError, "Failed to list subscriptions")
		return
	}
	defer rows.Close()

	answer := []Subscription{}
	for rows.Next() {
		var sub Subscription
		var dbStart time.Time
		var dbEnd sql.NullTime

		err := rows.Scan(&sub.ID, &sub.ServiceName, &sub.Price, &sub.UserID,
			&dbStart, &dbEnd, &sub.CreatedAt, &sub.UpdatedAt)
		if err != nil {
			logError("Failed to scan subscription row: %v", err)
			sendError(w, http.StatusInternalServerError, "Failed to read subscriptions")
			return
		}

		sub.StartDate = formatDate(dbStart)
		if dbEnd.Valid {
			sub.EndDate = formatDate(dbEnd.Time)
		}

		answer = append(answer, sub)
	}

	logInfo("Listed %d subscription(s)", len(answer))
	sendJSON(w, http.StatusOK, answer)
}

func updateSubscription(w http.ResponseWriter, r *http.Request, id string) {
	logDebug("updateSubscription: id=%s", id)

	if !isUUID(id) {
		sendError(w, http.StatusBadRequest, "Subscription ID must be a valid UUID")
		return
	}

	var req UpdateRequest
	if !readJSONBody(w, r, &req) {
		return
	}

	if req.ServiceName != "" {
		cleanName, err := cleanServiceName(req.ServiceName)
		if err != nil {
			sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		req.ServiceName = cleanName
	}

	if req.Price < 0 {
		sendError(w, http.StatusBadRequest, "Field 'price' must be greater than 0")
		return
	}

	logDebug("updateSubscription: reading current state for id=%s", id)

	var err error

	// Get the existing record so we can apply only the sent fields.
	var sub Subscription
	var dbStart time.Time
	var dbEnd sql.NullTime

	err = db.QueryRow(`
		SELECT id::text, service_name, price, user_id::text, start_date, end_date, created_at, updated_at
		FROM subscriptions
		WHERE id = $1::uuid`,
		id,
	).Scan(&sub.ID, &sub.ServiceName, &sub.Price, &sub.UserID,
		&dbStart, &dbEnd, &sub.CreatedAt, &sub.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		logDebug("updateSubscription: id=%s not found", id)
		sendError(w, http.StatusNotFound, "Subscription not found")
		return
	}
	if err != nil {
		logError("Failed to retrieve subscription %s for update: %v", id, err)
		sendError(w, http.StatusInternalServerError, "Failed to retrieve subscription")
		return
	}

	logDebug("updateSubscription: applying patch service_name=%q price=%d start_date=%q end_date=%q allow_behindhand_date=%v",
		req.ServiceName, req.Price, req.StartDate, req.EndDate, req.AllowBehindhandDate)

	if req.ServiceName != "" {
		sub.ServiceName = req.ServiceName
	}

	if req.Price > 0 {
		sub.Price = req.Price
	}

	if req.StartDate != "" {
		newStart, err := parseDate(req.StartDate)
		if err != nil {
			sendError(w, http.StatusBadRequest, "Field 'start_date' "+err.Error())
			return
		}
		if err := checkStartDateNotPast(newStart, req.AllowBehindhandDate); err != nil {
			sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		dbStart = newStart
	}

	var endParam interface{}
	if req.EndDate != "" {
		newEnd, err := parseDate(req.EndDate)
		if err != nil {
			sendError(w, http.StatusBadRequest, "Field 'end_date' "+err.Error())
			return
		}
		if newEnd.Before(dbStart) {
			sendError(w, http.StatusBadRequest, "Field 'end_date' must not be earlier than 'start_date'")
			return
		}
		endParam = newEnd
		dbEnd = sql.NullTime{Time: newEnd, Valid: true}
	} else if dbEnd.Valid {
		endParam = dbEnd.Time
	}

	logWarn("Updating subscription %s — modifies stored data", id)

	err = db.QueryRow(`
		UPDATE subscriptions
		SET service_name = $1, price = $2, start_date = $3, end_date = $4, updated_at = NOW()
		WHERE id = $5::uuid
		RETURNING updated_at`,
		sub.ServiceName, sub.Price, dbStart, endParam, id,
	).Scan(&sub.UpdatedAt)
	if err != nil {
		logError("Failed to update subscription %s: %v", id, err)
		sendError(w, http.StatusInternalServerError, "Failed to update subscription")
		return
	}

	sub.StartDate = formatDate(dbStart)
	sub.EndDate = ""
	if dbEnd.Valid {
		sub.EndDate = formatDate(dbEnd.Time)
	}

	logInfo("Updated subscription %s", id)
	sendJSON(w, http.StatusOK, sub)
}

func deleteSubscription(w http.ResponseWriter, id string) {
	logDebug("deleteSubscription: id=%s", id)

	if !isUUID(id) {
		sendError(w, http.StatusBadRequest, "Subscription ID must be a valid UUID")
		return
	}

	logWarn("Deleting subscription %s — destroys stored data", id)

	result, err := db.Exec(`DELETE FROM subscriptions WHERE id = $1::uuid`, id)
	if err != nil {
		logError("Failed to delete subscription %s: %v", id, err)
		sendError(w, http.StatusInternalServerError, "Failed to delete subscription")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		logDebug("deleteSubscription: id=%s not found", id)
		sendError(w, http.StatusNotFound, "Subscription not found")
		return
	}

	logInfo("Deleted subscription %s", id)
	w.WriteHeader(http.StatusNoContent)
}

func totalCostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	if from == "" || to == "" {
		sendError(w, http.StatusBadRequest, "Query parameters 'from' and 'to' are required")
		return
	}

	fromDate, err := parseDate(from)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Query parameter 'from' "+err.Error())
		return
	}

	toDate, err := parseDate(to)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Query parameter 'to' "+err.Error())
		return
	}

	if toDate.Before(fromDate) {
		sendError(w, http.StatusBadRequest, "Query parameter 'to' must not be before 'from'")
		return
	}

	userID := r.URL.Query().Get("user_id")
	serviceName := r.URL.Query().Get("service_name")

	if userID != "" && !isUUID(userID) {
		sendError(w, http.StatusBadRequest, "Query parameter 'user_id' must be a valid UUID")
		return
	}

	// end_date_calc_mode: filter by presence of end_date.
	//   "all"               — every overlapping subscription (default)
	//   "with_end_date"     — only subscriptions whose end_date is set
	//   "without_end_date"  — only open-ended subscriptions
	mode := r.URL.Query().Get("end_date_calc_mode")
	if mode == "" {
		mode = "all"
	}
	if mode != "all" && mode != "with_end_date" && mode != "without_end_date" {
		sendError(w, http.StatusBadRequest,
			"Query parameter 'end_date_calc_mode' must be one of: all, with_end_date, without_end_date")
		return
	}

	logDebug("totalCostHandler: period=%s..%s filters user_id=%q service_name=%q end_date_calc_mode=%q",
		from, to, userID, serviceName, mode)

	// Fetch only subscriptions whose period overlaps with [fromDate, toDate].
	rows, err := db.Query(`
		SELECT price, start_date, end_date
		FROM subscriptions
		WHERE start_date <= $2::date
		  AND (end_date IS NULL OR end_date >= $1::date)
		  AND ($3 = '' OR user_id::text = $3)
		  AND ($4 = '' OR service_name = $4)
		  AND ($5 = 'all'
		       OR ($5 = 'with_end_date'    AND end_date IS NOT NULL)
		       OR ($5 = 'without_end_date' AND end_date IS NULL))`,
		fromDate, toDate, userID, serviceName, mode)
	if err != nil {
		logError("Failed to fetch subscriptions for total: %v", err)
		sendError(w, http.StatusInternalServerError, "Failed to calculate total")
		return
	}
	defer rows.Close()

	total := 0
	count := 0
	for rows.Next() {
		var price int
		var start time.Time
		var end sql.NullTime

		err := rows.Scan(&price, &start, &end)
		if err != nil {
			logError("Failed to scan subscription for total: %v", err)
			sendError(w, http.StatusInternalServerError, "Failed to calculate total")
			return
		}

		endDate := toDate
		if end.Valid {
			endDate = end.Time
		}

		realStart := maxDate(start, fromDate)
		realEnd := minDate(endDate, toDate)
		months := countMonths(realStart, realEnd)
		contribution := price * months

		logDebug("totalCostHandler: subscription contributes %d (%d × %d months)", contribution, price, months)

		total += contribution
		count++
	}

	logInfo("Calculated total: %d across %d subscription(s) for period %s..%s (mode=%s)", total, count, from, to, mode)
	sendJSON(w, http.StatusOK, TotalAnswer{Total: total})
}

func parseDate(text string) (time.Time, error) {
	date, err := time.Parse("01-2006", text)
	if err != nil {
		return time.Time{}, errors.New("must be in MM-YYYY format")
	}
	return date, nil
}

func formatDate(date time.Time) string {
	return date.Format("01-2006")
}

func isUUID(text string) bool {
	return uuidRegex.MatchString(text)
}

func makeNewID() string {
	bytes := make([]byte, 16)
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}

	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return hex.EncodeToString(bytes[0:4]) + "-" +
		hex.EncodeToString(bytes[4:6]) + "-" +
		hex.EncodeToString(bytes[6:8]) + "-" +
		hex.EncodeToString(bytes[8:10]) + "-" +
		hex.EncodeToString(bytes[10:16])
}

func countMonths(start time.Time, end time.Time) int {
	years := end.Year() - start.Year()
	months := int(end.Month()) - int(start.Month())
	return years*12 + months + 1
}

func maxDate(a time.Time, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minDate(a time.Time, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func sendJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func sendError(w http.ResponseWriter, status int, text string) {
	sendJSON(w, status, ErrorAnswer{Error: text})
}

// readJSONBody enforces Content-Type, body size, no duplicate keys and no
// unknown fields, then decodes the body into dst. On any failure it sends
// the appropriate HTTP error itself and returns false.
func readJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		sendError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendError(w, http.StatusRequestEntityTooLarge, "Request body too large or unreadable")
		return false
	}

	if err := checkNoDuplicateKeys(body); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return false
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return false
	}
	if dec.More() {
		sendError(w, http.StatusBadRequest, "Body must contain a single JSON object")
		return false
	}
	return true
}

// checkNoDuplicateKeys walks the top-level JSON object and returns an error
// if a key appears more than once. encoding/json silently overwrites duplicates.
func checkNoDuplicateKeys(body []byte) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil {
		return errors.New("Invalid JSON body")
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		return errors.New("Body must be a JSON object")
	}

	seen := make(map[string]bool)
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return errors.New("Invalid JSON body")
		}
		key, ok := tok.(string)
		if !ok {
			return errors.New("Invalid JSON body")
		}
		if seen[key] {
			return fmt.Errorf("Duplicate field %q in request body", key)
		}
		seen[key] = true

		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return errors.New("Invalid JSON body")
		}
	}
	return nil
}

// cleanServiceName trims whitespace and validates the length range.
func cleanServiceName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("Field 'service_name' is required")
	}
	if len([]rune(s)) > maxServiceNameLen {
		return "", fmt.Errorf("Field 'service_name' must be at most %d characters", maxServiceNameLen)
	}
	return s, nil
}

// checkStartDateNotPast rejects start dates earlier than the current month
// unless allow is true (back-dating is explicitly opted in).
func checkStartDateNotPast(start time.Time, allow bool) error {
	if allow {
		return nil
	}
	now := time.Now().UTC()
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if start.Before(currentMonth) {
		return errors.New("Field 'start_date' cannot be earlier than the current month. " +
			"Set 'allow_behindhand_date': true to override.")
	}
	return nil
}
