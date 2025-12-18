// order-service/main.go
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

type Order struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Product   string    `json:"product"`
	Quantity  int       `json:"quantity"`
	Amount    float64   `json:"amount"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type OrderService struct {
	db                *sql.DB
	userServiceURL    string
	paymentServiceURL string
}

func NewOrderService(dbURL, userServiceURL, paymentServiceURL string) (*OrderService, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, err
	}

	return &OrderService{
		db:                db,
		userServiceURL:    userServiceURL,
		paymentServiceURL: paymentServiceURL,
	}, nil
}

// Service-to-service communication
func (s *OrderService) validateUser(userID int) error {
	url := fmt.Sprintf("%s/users/get?id=%d", s.userServiceURL, userID)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("user service unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("user not found")
	}

	return nil
}

func (s *OrderService) processPayment(orderID int, amount float64) error {
	payment := map[string]interface{}{
		"order_id": orderID,
		"amount":   amount,
	}

	paymentJSON, _ := json.Marshal(payment)
	url := fmt.Sprintf("%s/payments", s.paymentServiceURL)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(paymentJSON))
	if err != nil {
		return fmt.Errorf("payment service unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("payment failed")
	}

	return nil
}

func (s *OrderService) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var order Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate user exists (call user service)
	if err := s.validateUser(order.UserID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Create order
	order.Status = "pending"
	order.CreatedAt = time.Now()

	query := `INSERT INTO orders (user_id, product, quantity, amount, status, created_at) 
              VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	err := s.db.QueryRow(query,
		order.UserID, order.Product, order.Quantity,
		order.Amount, order.Status, order.CreatedAt).Scan(&order.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Process payment (call payment service)
	if err := s.processPayment(order.ID, order.Amount); err != nil {
		// Update order status to failed
		s.db.Exec("UPDATE orders SET status = $1 WHERE id = $2", "payment_failed", order.ID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update order status
	order.Status = "completed"
	s.db.Exec("UPDATE orders SET status = $1 WHERE id = $2", order.Status, order.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	userServiceURL := os.Getenv("USER_SERVICE_URL")
	paymentServiceURL := os.Getenv("PAYMENT_SERVICE_URL")

	service, err := NewOrderService(dbURL, userServiceURL, paymentServiceURL)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/orders", service.CreateOrder)

	log.Println("Order service starting on :8082")
	log.Fatal(http.ListenAndServe(":8082", mux))
}
