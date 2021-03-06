package acme

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// HTTPProviderServer implements ChallengeProvider for `http-01` challenge
// It may be instantiated without using the NewHTTPProviderServer function if
// you want only to use the default values.
type HTTPProviderServer struct {
	iface               string
	port                string
	done                chan bool
	listener            net.Listener
	allowXForwardedHost bool
}

// NewHTTPProviderServer creates a new HTTPProviderServer on the selected interface and port.
// Setting iface and / or port to an empty string will make the server fall back to
// the "any" interface and port 80 respectively.
func NewHTTPProviderServer(iface, port string, allowXForwardedHost bool) *HTTPProviderServer {
	return &HTTPProviderServer{iface: iface, port: port, allowXForwardedHost: allowXForwardedHost}
}

// Present starts a web server and makes the token available at `HTTP01ChallengePath(token)` for web requests.
func (s *HTTPProviderServer) Present(domain, token, keyAuth string) error {
	if s.port == "" {
		s.port = "80"
	}

	var err error
	s.listener, err = net.Listen("tcp", net.JoinHostPort(s.iface, s.port))
	if err != nil {
		return fmt.Errorf("Could not start HTTP server for challenge -> %v", err)
	}

	s.done = make(chan bool)
	go s.serve(domain, token, keyAuth)
	return nil
}

// CleanUp closes the HTTP server and removes the token from `HTTP01ChallengePath(token)`
func (s *HTTPProviderServer) CleanUp(domain, token, keyAuth string) error {
	if s.listener == nil {
		return nil
	}
	s.listener.Close()
	<-s.done
	return nil
}

func (s *HTTPProviderServer) serve(domain, token, keyAuth string) {
	path := HTTP01ChallengePath(token)

	// The handler validates the HOST header and request type.
	// For validation it then writes the token the server returned with the challenge
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		requestHost := r.Host
		if s.allowXForwardedHost {
			xForwardedHost := r.Header.Get("x-forwarded-host")
			if xForwardedHost != "" {
				requestHost = xForwardedHost
			}
		}

		if strings.HasPrefix(requestHost, domain) && r.Method == "GET" {
			w.Header().Add("Content-Type", "text/plain")
			w.Write([]byte(keyAuth))
			logf("[INFO][%s] Served key authentication", domain)
		} else {
			logf("[WARN] Received request for domain %s with method %s but the domain did not match any challenge. Please ensure your are passing the HOST header properly.", r.Host, r.Method)
			w.Write([]byte("TEST"))
		}
	})

	httpServer := &http.Server{
		Handler: mux,
	}
	// Once httpServer is shut down we don't want any lingering
	// connections, so disable KeepAlives.
	httpServer.SetKeepAlivesEnabled(false)
	httpServer.Serve(s.listener)
	s.done <- true
}
