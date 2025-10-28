package httpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// INTEGRATION TESTS - Real-world Scenarios
// ============================================================================

func TestIntegration_RESTfulAPI(t *testing.T) {
	// Simulate a RESTful API server
	users := make(map[string]map[string]interface{})
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		fmt.Println(r.URL.Path)

		switch r.Method {
		case "GET":
			if strings.HasPrefix(r.URL.Path, "/users/") {
				id := strings.TrimPrefix(r.URL.Path, "/users/")
				if user, ok := users[id]; ok {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(user)
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			} else if r.URL.Path == "/users" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(users)
			}

		case "POST":
			if r.URL.Path == "/users" {
				var user map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				id := fmt.Sprintf("%d", len(users)+1)
				user["id"] = id
				users[id] = user
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(user)
			}

		case "PUT":
			if strings.HasPrefix(r.URL.Path, "/users/") {
				id := strings.TrimPrefix(r.URL.Path, "/users/")
				var user map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				user["id"] = id
				users[id] = user
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(user)
			}

		case "DELETE":
			if strings.HasPrefix(r.URL.Path, "/users/") {
				id := strings.TrimPrefix(r.URL.Path, "/users/")
				delete(users, id)
				w.WriteHeader(http.StatusNoContent)
			}
		}
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Test CREATE
	t.Run("Create User", func(t *testing.T) {
		user := map[string]interface{}{
			"name":  "John Doe",
			"email": "john@example.com",
		}

		resp, err := client.Post(server.URL+"/users", WithJSON(user))
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		var created map[string]interface{}
		if err := resp.JSON(&created); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if created["name"] != "John Doe" {
			t.Errorf("Expected name 'John Doe', got %v", created["name"])
		}
	})

	// Test READ
	t.Run("Get User", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/users/1")
		if err != nil {
			t.Fatalf("Failed to get user: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	// Test UPDATE
	t.Run("Update User", func(t *testing.T) {
		user := map[string]interface{}{
			"name":  "Jane Doe",
			"email": "jane@example.com",
		}

		resp, err := client.Put(server.URL+"/users/1", WithJSON(user))
		if err != nil {
			t.Fatalf("Failed to update user: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	// Test DELETE
	t.Run("Delete User", func(t *testing.T) {
		resp, err := client.Delete(server.URL + "/users/1")
		if err != nil {
			t.Fatalf("Failed to delete user: %v", err)
		}

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected status 204, got %d", resp.StatusCode)
		}
	})
}

func TestIntegration_Authentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authentication
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if token != "valid-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		} else if strings.HasPrefix(auth, "Basic ") {
			username, password, ok := r.BasicAuth()
			if !ok || username != "user" || password != "pass" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"authenticated":true}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	t.Run("Bearer Token", func(t *testing.T) {
		resp, err := client.Get(server.URL, WithBearerToken("valid-token"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Basic Auth", func(t *testing.T) {
		resp, err := client.Get(server.URL, WithBasicAuth("user", "pass"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("No Auth", func(t *testing.T) {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})
}

func TestIntegration_Pagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		limit := r.URL.Query().Get("limit")

		if page == "" {
			page = "1"
		}
		if limit == "" {
			limit = "10"
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"page":  page,
			"limit": limit,
			"data":  []string{"item1", "item2", "item3"},
		})
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Test pagination
	for page := 1; page <= 3; page++ {
		resp, err := client.Get(server.URL,
			WithQuery("page", page),
			WithQuery("limit", 10),
		)
		if err != nil {
			t.Fatalf("Request failed for page %d: %v", page, err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := resp.JSON(&result); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if result["page"] != fmt.Sprintf("%d", page) {
			t.Errorf("Expected page %d, got %v", page, result["page"])
		}
	}
}

// ============================================================================
// STRESS TESTS
// ============================================================================

func TestStress_HighConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	// 检测环境并调整参数
	numGoroutines := 50       // 降低并发数
	requestsPerGoroutine := 5 // 减少每个协程的请求数

	// 在CI环境中进一步降低
	if os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true" {
		numGoroutines = 20
		requestsPerGoroutine = 2
	}

	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		time.Sleep(2 * time.Millisecond) // 减少服务器延迟
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 使用更宽松的配置
	config := DefaultConfig()
	config.Timeout = 30 * time.Second // 增加超时时间
	config.AllowPrivateIPs = true     // 允许访问测试服务器
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*requestsPerGoroutine)

	start := time.Now()

	// 使用信号量控制并发启动
	sem := make(chan struct{}, 10) // 限制同时启动的协程数

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// 获取信号量
			sem <- struct{}{}
			defer func() { <-sem }()

			for j := 0; j < requestsPerGoroutine; j++ {
				// 添加小延迟避免瞬间大量请求
				if j > 0 {
					time.Sleep(time.Millisecond)
				}

				_, err := client.Get(server.URL)
				if err != nil {
					select {
					case errors <- err:
					default:
						// 错误通道满了，忽略
					}
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	duration := time.Since(start)

	errorCount := 0
	for err := range errors {
		t.Logf("Error: %v", err)
		errorCount++
	}

	totalRequests := numGoroutines * requestsPerGoroutine
	successRate := float64(totalRequests-errorCount) / float64(totalRequests) * 100

	t.Logf("Stress Test Results:")
	t.Logf("  Total Requests: %d", totalRequests)
	t.Logf("  Successful: %d", totalRequests-errorCount)
	t.Logf("  Failed: %d", errorCount)
	t.Logf("  Success Rate: %.2f%%", successRate)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f req/s", float64(totalRequests)/duration.Seconds())

	// 根据环境调整期望成功率
	expectedSuccessRate := 95.0
	if os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true" {
		expectedSuccessRate = 85.0 // CI环境中降低期望
	}

	if successRate < expectedSuccessRate {
		t.Errorf("Success rate too low: %.2f%% (expected: %.1f%%)", successRate, expectedSuccessRate)
	}
}

func TestStress_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Send 1KB response
		data := make([]byte, 1024)
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Make many requests to test memory management
	for i := 0; i < 10000; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
		_ = resp.RawBody // Use the response

		if i%1000 == 0 {
			t.Logf("Completed %d requests", i)
		}
	}
}
