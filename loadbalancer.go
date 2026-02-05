package main

import (
	"fmt"
	"net/http/httputil"
	"net/http"
	"net/url"
	"sync"
)

func main(){
	backend1, _ := url.Parse("http://localhost:8081")
    backend2, _ := url.Parse("http://localhost:8082")
    backend3, _ := url.Parse("http://localhost:8083")

    proxy1 := httputil.NewSingleHostReverseProxy(backend1)
    proxy2 := httputil.NewSingleHostReverseProxy(backend2)
    proxy3 := httputil.NewSingleHostReverseProxy(backend3)

	current:=0
	var mux sync.Mutex

	http.HandleFunc("/",func(w http.ResponseWriter, r *http.Request){
		mux.Lock()
		if current==0 {
			fmt.Println("Forwarding to backend on port 8081")
            proxy1.ServeHTTP(w, r)
		}else if current==1 {
			fmt.Println("Forwarding to backend on port 8082")
            proxy2.ServeHTTP(w, r)
		}else {
			fmt.Println("Forwarding to backend on port 8083")
            proxy3.ServeHTTP(w, r)
		}
		current=(current+1)%3
		mux.Unlock()
	})
	fmt.Println("Load balancer starting on port 8080...")
	http.ListenAndServe(":8080", nil)
}
