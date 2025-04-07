package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func main() {
	// Create router
	router := mux.NewRouter()

	// Create reverse proxy to httpbin.org
	upstreamURL, _ := url.Parse("https://httpbin.org")
	proxy := httputil.NewSingleHostReverseProxy(upstreamURL)

	// Configure proxy transport with HTTP/2 support
	transport := &http.Transport{}
	http2.ConfigureTransport(transport)
	proxy.Transport = transport

	// Modify the director function to adjust the request for httpbin
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = upstreamURL.Host
		req.URL.Path = "/anything"
	}

	// Define auth middleware and proxy handler
	router.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("in")
		// Detect if it's a gRPC request
		isGrpc := r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc")

		var token string
		if isGrpc {
			// Extract token from gRPC metadata
			log.Println("Handling gRPC request")
			token = r.Header.Get("authorization")
			// Remove 'Bearer ' if present
			token = strings.TrimPrefix(token, "Bearer ")

			// For gRPC, handle the request differently as httpbin won't work
			if validateToken(token) {
				// Valid token for gRPC
				log.Printf("Valid gRPC token, returning success")
				w.Header().Set("Content-Type", "application/grpc")
				w.Header().Set("Grpc-Status", "0") // Success
				w.WriteHeader(http.StatusOK)
				// ... implement grpc proxy here
				// no grpcbin...
			} else {
				// Invalid token for gRPC
				log.Printf("Invalid token: %s", token)
				w.Header().Set("Content-Type", "application/grpc")
				w.Header().Set("Grpc-Status", "16") // Unauthenticated
				w.Header().Set("Grpc-Message", "Invalid authentication")
				w.WriteHeader(http.StatusUnauthorized)
			}
			return
		} else {
			// Extract token from HTTP cookie
			log.Println("Handling HTTP/1.1 request")
			cookie, err := r.Cookie("auth_token")
			if err == nil {
				token = cookie.Value
			} else {
				// For httpbin testing, also check Authorization header for HTTP requests
				authHeader := r.Header.Get("Authorization")
				if authHeader != "" {
					token = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			// Validate token for HTTP requests
			if !validateToken(token) {
				log.Printf("Invalid token: %s", token)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			log.Printf("Valid token, proxying request to httpbin.org")

			// Add headers ...
			r.Header.Set("X-Authenticated-User", getUserFromToken(token))

			// Proxy the HTTP request
			proxy.ServeHTTP(w, r)
		}
	})

	h2s := &http2.Server{}
	h1s := &http.Server{
		Addr:    ":8080",
		Handler: h2c.NewHandler(router, h2s),
	}

	log.Println("Starting proxy server on :8080...")
	log.Fatal(h1s.ListenAndServe())
}

// Mock token validation, could instead proxy upstream to KRATOS / HYDRA / ...
func validateToken(token string) bool {
	return token != ""
}

// Mock user extraction
func getUserFromToken(token string) string {
	return "user-123"
}
