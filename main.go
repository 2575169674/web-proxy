package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

var (
	proxyPort  int    // 用于二次代理转发的端口
	directPort int    // 用于直接转发的端口
	proxyURL   string // 第二级代理服务器URL
)

func init() {
	flag.IntVar(&proxyPort, "proxy-port", 9522, "用于二次代理转发的监听端口")
	flag.IntVar(&directPort, "direct-port", 9521, "用于直接转发的监听端口")
	flag.StringVar(&proxyURL, "proxy-url", "", "第二级代理服务器URL，例如 127.0.0.1:8080")
	flag.Parse()
}

// parseProxyURL 解析代理服务器的URL，提取认证信息，并返回代理地址和认证头 PS: 注意该解析只能解析 账户:密码@服务器:端口
func parseProxyURL(proxyURL string) (string, string, error) {
	data := strings.Split(proxyURL, "@")
	var (
		user   string
		server string
	)
	if len(data) == 2 {
		user = data[0]
		server = data[1]
	} else if len(data) == 1 {
		server = data[0]
	}
	return server, user, nil
}

// 创建一个代理配置用于第二级代理的http.Transport
var proxyTransport = &http.Transport{
	Proxy: func(_ *http.Request) (*url.URL, error) {
		proxyStr, _, err := parseProxyURL(proxyURL)
		if err != nil {
			log.Println("Error parsing proxy URL:", err)
			return nil, err
		}
		return url.Parse(proxyStr)
	},
	TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // 如果第二级代理使用自签名证书，需要跳过证书验证
}

// handleProxyTunneling 处理通过第二级代理转发的HTTPS隧道请求
func handleProxyTunneling(w http.ResponseWriter, r *http.Request) {
	proxyStr, auth, err := parseProxyURL(proxyURL)
	if err != nil {
		http.Error(w, "Failed to parse proxy URL", http.StatusInternalServerError)
		return
	}

	// 连接到第二级代理服务器
	proxyConn, err := net.Dial("tcp", proxyStr)
	if err != nil {
		http.Error(w, "Failed to connect to the second proxy", http.StatusServiceUnavailable)
		return
	}

	// 如果需要认证，设置代理服务器的认证信息
	authorizationHeader := ""
	if auth != "" {
		encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
		authorizationHeader = "Proxy-Authorization: Basic " + encodedAuth + "\r\n"
	}

	// 发送CONNECT请求给第二级代理
	connectRequest := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n%s\r\n", r.Host, r.Host, authorizationHeader)
	proxyConn.Write([]byte(connectRequest))
	resp, err := http.ReadResponse(bufio.NewReader(proxyConn), r)
	if err != nil {
		http.Error(w, "Failed to read response from the second proxy", http.StatusServiceUnavailable)
		return
	}
	if resp.StatusCode != 200 {
		http.Error(w, "Failed to connect to the host through the second proxy", resp.StatusCode)
		return
	}

	// 响应客户端的CONNECT请求，发送200 OK状态码
	w.WriteHeader(http.StatusOK)

	// 劫持连接
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	// 从此点开始，不要再使用w来写入响应

	// 开始转发数据
	go transfer(clientConn, proxyConn)
	go transfer(proxyConn, clientConn)
}

// handleDirectTunneling 处理直接转发的HTTPS隧道请求
func handleDirectTunneling(w http.ResponseWriter, r *http.Request) {
	// 直接连接目标服务器
	destConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, "Failed to connect to the host", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	// 开始转发数据
	go transfer(destConn, clientConn)
	go transfer(clientConn, destConn)
}

// handleProxyHTTP 处理通过第二级代理转发的HTTP请求
func handleProxyHTTP(w http.ResponseWriter, r *http.Request) {
	// 使用配置了第二级代理的http.Transport发送请求
	proxy := httputil.NewSingleHostReverseProxy(nil)
	proxy.Transport = proxyTransport
	proxy.ServeHTTP(w, r)
}

// handleDirectHTTP 处理直接转发的HTTP请求
func handleDirectHTTP(w http.ResponseWriter, r *http.Request) {
	// 使用默认的http.Transport发送请求
	proxy := httputil.NewSingleHostReverseProxy(nil)
	proxy.ServeHTTP(w, r)
}

// transfer 转发数据
func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}

// logRequest Log日志
func logRequest(r *http.Request, title string) {
	log.Printf("[%s] 请求: %s %s %s", title, r.Method, r.Host, r.RequestURI)
	if r.TLS != nil {
		log.Println("[" + title + "] 安全连接: TLS已启用")
	} else {
		log.Println("[" + title + "]安全连接: TLS未启用")
	}
}

func main() {
	// 启动HTTP服务（二次代理转发）
	go func() {
		proxy := &http.Server{
			Addr: fmt.Sprintf(":%d", proxyPort),
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				logRequest(r, "二次代理")
				if r.Method == http.MethodConnect {
					handleProxyTunneling(w, r)
				} else {
					handleProxyHTTP(w, r)
				}
			}),
		}
		log.Fatal(proxy.ListenAndServe())
	}()

	// 启动HTTP服务（直接转发）
	go func() {
		direct := &http.Server{
			Addr: fmt.Sprintf(":%d", directPort),
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				logRequest(r, "正向代理")
				if r.Method == http.MethodConnect {
					handleDirectTunneling(w, r)
				} else {
					handleDirectHTTP(w, r)
				}
			}),
		}
		log.Fatal(direct.ListenAndServe())
	}()

	// 阻塞主goroutine
	select {}
}
