# Error Handling Standards

This document defines how we handle, wrap, and inspect errors across our Go codebases. Following these standards ensures consistent logs and predictable error handling behavior.

---

## 1. Golden Rules
* **Never swallow errors.** Always log, wrap, or return them.
* **Context is king.** Every time an error moves up an architectural layer, add context using wrapping if it aids debugging.
* **Sanitize external errors.** Never expose internal database or system errors directly to the API client.

---

## 2. When to Use Wrapping (`%w`)
Use `fmt.Errorf("...: %w", err)` when an error crosses architectural boundaries (e.g., DB -> Service -> Transport) and the caller up the chain might need to programmatically inspect the root cause.

### Standard Layered Pattern:
1. **Infra Layer:** Returns a raw sentinel error or domain error (e.g., `db.ErrNotFound`).
2. **Business Layer:** Wraps the error to add business context (e.g., `fmt.Errorf("failed to fetch user %s: %w", userID, err)`).
3. **API Layer:** Uses `errors.Is` or `errors.As` to decide the HTTP status code, then logs the full error chain.

```go
// Good Practice: Adding relevant context while preserving the underlying error
if err != nil {
    return fmt.Errorf("failed to process payment for order %s: %w", orderID, err)
}
```

## Example
```go
package user

import (
	"errors"
	"fmt"
)

// 1. Infrastructure Layer (e.g., database)
var ErrUserNotFound = errors.New("user not found in DB")

func (r *Repository) FetchUser(id string) (*User, error) {
	// If database fails, return the base sentinel error
	return nil, ErrUserNotFound
}

// 2. Service Layer (Business Logic)
func (s *Service) GetUserProfile(id string) (*Profile, error) {
	user, err := s.repo.FetchUser(id)
	if err != nil {
		// WRAP IT: Add business context (what were we doing? who failed?)
		return nil, fmt.Errorf("failed to load profile for user %s: %w", id, err)
	}
	return &Profile{Name: user.Name}, nil
}

// 3. Transport Layer (HTTP Handler)
func (c *Controller) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	profile, err := c.service.GetUserProfile(id)
	
	if err != nil {
		// WRAP IT: Add the top-level application context for logging
		finalErr := fmt.Errorf("http handler failure: %w", err)
		
		// This prints: "LOG: http handler failure: failed to load profile for user 123: user not found in DB"
		log.Println("LOG:", finalErr) 

		// Inspect the root cause to send the correct HTTP response
		if errors.Is(finalErr, ErrUserNotFound) {
			http.Error(w, "User profile does not exist", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	
	json.NewEncoder(w).Encode(profile)
}
```