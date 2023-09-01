package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
)

func main() {
	var addr = flag.String("addr", "127.0.0.1:8080", "The addr of the proxy.")
	flag.Parse()
	proxy := NewProxyHttpServer1()
	log.Println("Starting proxy server on", *addr)
	log.Fatal(http.ListenAndServe(*addr, proxy))
}

// NewProxyHttpServer creates and returns a proxy server
func NewProxyHttpServer1() *ProxyHttpServer {
	proxy := ProxyHttpServer{}
	return &proxy
}

type ProxyHttpServer struct{}

func (proxy *ProxyHttpServer) handleHttps(w http.ResponseWriter, r *http.Request) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	// Hijack 可以将 HTTP 对应的 TCP 连接取出
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// 解析目标服务器的地址
	host := r.URL.Host
	if host == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	log.Printf("Accepting CONNECT to %v\n", host)

	// 建立与目标服务器的连接
	serverConn, err := net.Dial("tcp", host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer serverConn.Close()

	// 告诉客户端与目标服务器连接成功
	clientConn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))

	// 启动双向数据传输
	go func() {
		_, err := io.Copy(serverConn, clientConn)
		if err != nil {
			log.Printf("Error copying from client to server: %v", err)
		}
	}()

	_, err = io.Copy(clientConn, serverConn)
	if err != nil {
		log.Printf("Error copying from server to client: %v", err)
	}

	log.Println("Complete communication")
}

func (proxy *ProxyHttpServer) handleHttp(w http.ResponseWriter, r *http.Request) {
	log.Printf("Got request %v %v %v %v", r.URL.Path, r.Host, r.Method, r.URL.String())

	// 创建一个HTTP客户端
	client := &http.Client{}

	// 创建一个新的请求，将原始请求的信息复制到新请求中
	newRequest, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
	if err != nil {
		http.Error(w, "Error creating new request", http.StatusInternalServerError)
		return
	}
	newRequest.Header = r.Header

	// 发送新请求到目标服务器
	resp, err := client.Do(newRequest)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error sending request: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// 复制目标服务器的响应到代理服务器的响应
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	log.Printf("Copying resp to client %v [%d]\n", resp.Status, resp.StatusCode)
	w.WriteHeader(resp.StatusCode)
	len, err := io.Copy(w, resp.Body)
	log.Printf("Copied %v bytes to client error=%v", len, err)
}

func (proxy *ProxyHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "CONNECT" {
		proxy.handleHttps(w, r)
		return
	}
	proxy.handleHttp(w, r)
}
