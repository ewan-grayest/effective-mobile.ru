package main

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

var db *sql.DB

func main() {
	// Load .env first so initLogger sees LOG_LEVEL from it.
	err := godotenv.Load()
	if err != nil {
		log.Println("[INFO] No .env file found, using environment variables")
	}

	initLogger()

	// Connect to the database.
	dsn := "host=" + getEnv("DB_HOST", "localhost") +
		" port=" + getEnv("DB_PORT", "5432") +
		" user=" + getEnv("DB_USER", "postgres") +
		" password=" + getEnv("DB_PASSWORD", "postgres") +
		" dbname=" + getEnv("DB_NAME", "subscriptions") +
		" sslmode=" + getEnv("DB_SSLMODE", "disable")

	logDebug("Opening database connection: host=%s port=%s db=%s",
		getEnv("DB_HOST", "localhost"), getEnv("DB_PORT", "5432"), getEnv("DB_NAME", "subscriptions"))

	db, err = sql.Open("postgres", dsn)
	if err != nil {
		logError("Failed to open database: %v", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		logError("Failed to connect to database: %v", err)
		os.Exit(1)
	}
	logInfo("Connected to database")

	// Run migrations.
	dbURL := "postgres://" + getEnv("DB_USER", "postgres") + ":" +
		getEnv("DB_PASSWORD", "postgres") + "@" +
		getEnv("DB_HOST", "localhost") + ":" +
		getEnv("DB_PORT", "5432") + "/" +
		getEnv("DB_NAME", "subscriptions") + "?sslmode=" +
		getEnv("DB_SSLMODE", "disable")

	logDebug("Initializing migrations from file://migrations")
	m, err := migrate.New("file://migrations", dbURL)
	if err != nil {
		logError("Failed to initialize migrations: %v", err)
		os.Exit(1)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		logError("Failed to run migrations: %v", err)
		os.Exit(1)
	}
	logInfo("Migrations applied")

	// Routes — every API handler is wrapped with withLogging so each request
	// gets entry/exit log lines.
	http.HandleFunc("/api/v1/subscriptions/total", withLogging(totalCostHandler))
	http.HandleFunc("/api/v1/subscriptions", withLogging(subscriptionsHandler))
	http.HandleFunc("/api/v1/subscriptions/", withLogging(oneSubscriptionHandler))
	http.HandleFunc("/swagger/doc.json", swaggerJSONHandler)
	http.HandleFunc("/swagger/", swaggerUIHandler)
	http.HandleFunc("/swagger", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/", http.StatusMovedPermanently)
	})

	port := getEnv("SERVER_PORT", "8080")
	logInfo("Server started on port %s", port)
	logInfo("Swagger UI: http://localhost:%s/swagger/", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logError("HTTP server error: %v", err)
		os.Exit(1)
	}
}

func getEnv(name string, backup string) string {
	value := os.Getenv(name)
	if value == "" {
		return backup
	}
	return value
}

func swaggerJSONHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./docs/swagger.json")
}

func swaggerUIHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
  <title>Subscription Service API</title>
  <meta charset="utf-8"/>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"/>
</head>
<body>
<div id="swagger-ui"></div>
<script>
  SwaggerUIBundle({
    url: "/swagger/doc.json",
    dom_id: "#swagger-ui",
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
    layout: "BaseLayout"
  })
</script>
</body>
</html>`))
}
