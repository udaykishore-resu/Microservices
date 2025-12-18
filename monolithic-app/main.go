package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	_ "github.com/lib/pq"
)

// Single database connection for entire application
var db *sql.DB

// User domain
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Order domain
type Order struct {
	ID      int     `json:"id"`
	UserID  int     `json:"user_id"`
	Product string  `json:"product"`
	Amount  float64 `json:"amount"`
}

// Payment domain
type Payment struct {
	ID      int     `json:"id"`
	OrderID int     `json:"order_id"`
	Amount  float64 `json:"amount"`
	Status  string  `json:"status"`
}

// All handlers in same application
func createUserHandler(w http.ResponseWriter, r *http.Request) {
	var user User
	json.NewDecoder(r.Body).Decode(&user)

	// Direct database access
	_, err := db.Exec("INSERT INTO users (name, email) VALUES ($1, $2)",
		user.Name, user.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(user)
}

func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	var order Order
	json.NewDecoder(r.Body).Decode(&order)

	// Direct method call within same application
	user := getUserByID(order.UserID)
	if user == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Process order
	_, err := db.Exec("INSERT INTO orders (user_id, product, amount) VALUES ($1, $2, $3)",
		order.UserID, order.Product, order.Amount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Process payment in same transaction
	processPayment(order.ID, order.Amount)

	json.NewEncoder(w).Encode(order)
}

func processPayment(orderID int, amount float64) error {
	// Payment processing logic
	_, err := db.Exec("INSERT INTO payments (order_id, amount, status) VALUES ($1, $2, $3)",
		orderID, amount, "completed")
	return err
}

func getUserByID(userID int) *User {
	var user User
	err := db.QueryRow("SELECT id, name, email FROM users WHERE id = $1", userID).
		Scan(&user.ID, &user.Name, &user.Email)
	if err != nil {
		return nil
	}
	return &user
}

func main() {
	var err error
	// Single database connection
	db, err = sql.Open("postgres", "postgres://user:pass@localhost/monolith?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// All routes in single server
	http.HandleFunc("/users", createUserHandler)
	http.HandleFunc("/orders", createOrderHandler)

	log.Println("Monolithic server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
