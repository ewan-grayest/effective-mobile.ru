package main

import "time"

type Subscription struct {
	ID          string    `json:"id"`
	ServiceName string    `json:"service_name"`
	Price       int       `json:"price"`
	UserID      string    `json:"user_id"`
	StartDate   string    `json:"start_date"`
	EndDate     string    `json:"end_date,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateRequest struct {
	ServiceName         string `json:"service_name"`
	Price               int    `json:"price"`
	UserID              string `json:"user_id"`
	StartDate           string `json:"start_date"`
	EndDate             string `json:"end_date"`
	AllowBehindhandDate bool   `json:"allow_behindhand_date"`
}

type UpdateRequest struct {
	ServiceName         string `json:"service_name"`
	Price               int    `json:"price"`
	StartDate           string `json:"start_date"`
	EndDate             string `json:"end_date"`
	AllowBehindhandDate bool   `json:"allow_behindhand_date"`
}

type ErrorAnswer struct {
	Error string `json:"error"`
}

type TotalAnswer struct {
	Total int `json:"total"`
}
