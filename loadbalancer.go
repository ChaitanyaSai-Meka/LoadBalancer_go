package main

import (
	"net/http/httputil"
	"os"
	"net/http"
	"net/url"
	"sync"
	"log"
	"time"
	"strings"
	"github.com/joho/godotenv"
)

type Backend struct {
	URL     string
	Proxy   *httputil.ReverseProxy
	Alive   bool
	mux     sync.RWMutex
}

func (b *Backend) SetAlive(alive bool) {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.Alive = alive
}

func (b *Backend) IsAlive() bool {
	b.mux.RLock()
	defer b.mux.RUnlock()
	return b.Alive
}

type LoadBalancer struct {
	backends []*Backend
	current  int
	mux      sync.Mutex
}

func NewLoadBalancer(backendURLs []string) *LoadBalancer {
	lb := &LoadBalancer{
		backends: []*Backend{},
		current:  0,
	}
	
	for _, backendURL := range backendURLs {
		parsedURL, err := url.Parse(backendURL)
		
		if err != nil {
			log.Printf("[ERROR] Failed to parse URL %s: %v\n", backendURL, err)
			continue
		}
		
		proxy := httputil.NewSingleHostReverseProxy(parsedURL)
		
		backend := &Backend{
			URL:   backendURL,
			Proxy: proxy,
			Alive: true,
		}
		lb.backends = append(lb.backends, backend)
		log.Printf("[INFO] Added backend: %s\n", backendURL)
	}
	
	return lb
}

func (lb *LoadBalancer) getNextBackend() *Backend {
	lb.mux.Lock()
	defer lb.mux.Unlock()
	
	for i := 0; i < len(lb.backends); i++ {
		idx := (lb.current + i) % len(lb.backends)
		
		if lb.backends[idx].IsAlive() {
			lb.current = (idx + 1) % len(lb.backends)
			return lb.backends[idx]
		}
	}
	
	return nil
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()  
	
	selectedBackend := lb.getNextBackend()
	
	if selectedBackend == nil {
		log.Printf("[ERROR] All backends are down - Request: %s %s\n", r.Method, r.URL.Path)
		http.Error(w, "Service unavailable - all backends are down", http.StatusServiceUnavailable)
		return
	}
	
	log.Printf("[INFO] Forwarding request to %s - Path: %s %s\n", 
		selectedBackend.URL, r.Method, r.URL.Path)
	
	selectedBackend.Proxy.ServeHTTP(w, r)
	
	duration := time.Since(start)
	log.Printf("[INFO] Request completed in %v - Backend: %s\n", duration, selectedBackend.URL)
}

func (lb *LoadBalancer) healthCheck() {
	log.Println("[INFO] Running health checks...")
	
	aliveCount := 0
	for _, backend := range lb.backends {
		resp, err := http.Get(backend.URL)
		
		if err != nil {
			log.Printf("[WARN] Health check failed for %s: %v\n", backend.URL, err)
			backend.SetAlive(false)
		} else if resp.StatusCode != http.StatusOK {
			log.Printf("[WARN] Backend %s returned status %d\n", backend.URL, resp.StatusCode)
			backend.SetAlive(false)
		} else {
			if !backend.IsAlive() {
				log.Printf("[INFO] Backend %s is now UP (recovered)\n", backend.URL)
			}
			backend.SetAlive(true)
			aliveCount++
		}
		
		if resp != nil {
			resp.Body.Close()
		}
	}
	
	log.Printf("[INFO] Health check complete: %d/%d backends alive\n", aliveCount, len(lb.backends))
}

func (lb *LoadBalancer) startHealthChecks(interval time.Duration) {
	log.Printf("[INFO] Starting health checks (interval: %v)\n", interval)
	
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			lb.healthCheck()
		}
	}()
}

func (lb *LoadBalancer) getStats() {
	aliveCount := 0
	for _, backend := range lb.backends {
		if backend.IsAlive() {
			aliveCount++
		}
	}
	
	log.Printf("[STATS] Total backends: %d, Alive: %d, Down: %d\n", 
		len(lb.backends), aliveCount, len(lb.backends)-aliveCount)
}

func main(){

	en := godotenv.Load()
	if en != nil {
		log.Println("[WARN] No .env file found, using system environment variables")
	}
	
	Port:=os.Getenv("PORT")
	backendsEnv:=os.Getenv("Backend_URLs")

	if backendsEnv == "" {
		log.Fatal("Backend_URLs environment variable not set")
	}
	if Port == "" {
		log.Fatal("PORT environment variable not set")
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)  
	
	log.Println("[INFO] Starting load balancer...")
	
	backendURLs := strings.Split(backendsEnv, ",")
	
	lb := NewLoadBalancer(backendURLs)
	
	if len(lb.backends) == 0 {
		log.Fatal("[FATAL] No valid backend servers configured!")
	}

	lb.healthCheck()
	
	lb.startHealthChecks(10 * time.Second)
	
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			lb.getStats()
		}
	}()
	
	log.Printf("[INFO] Load balancer listening on :%s\n", Port)
	log.Printf("[INFO] Configured %d backend servers\n", len(lb.backends))
	
	err := http.ListenAndServe(":"+Port, lb)
	if err != nil {
		log.Fatalf("[FATAL] Server failed to start: %v\n", err)
	}
}