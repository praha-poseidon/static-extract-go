package main

import "net/http"

func main() {
	http.HandleFunc("/api/users", users)
	http.HandleFunc("/api/health", health)
	_ = http.ListenAndServe(":8080", nil)
}

func users(w http.ResponseWriter, r *http.Request) {}
func health(w http.ResponseWriter, r *http.Request) {}
