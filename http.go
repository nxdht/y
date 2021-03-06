package y

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	httpDefaultReadTimeout  = 30
	httpDefaultWriteTimeout = 30
)

type HttpServer struct {
	server *http.Server
	ctx    context.Context
	cancel context.CancelFunc

	caFile   string
	certFile string
	keyFile  string
}

type HttpServerConfig struct {
	ListenPort   int
	ReadTimeout  int
	WriteTimeout int
	CaFile       string
	CertFile     string
	KeyFile      string
}

func NewHttpServer(ctx context.Context, config *HttpServerConfig) (server *HttpServer, err error) {
	if config == nil {
		return nil, errors.New("nil parameter config")
	}
	if config.ListenPort <= 0 {
		return nil, errors.New("invalid listen port")
	}

	readTimeout := httpDefaultReadTimeout
	if config.ReadTimeout > 0 {
		readTimeout = config.ReadTimeout
	}

	writeTimeout := httpDefaultWriteTimeout
	if config.WriteTimeout > 0 {
		writeTimeout = config.WriteTimeout
	}

	ctx, cancel := context.WithCancel(ctx)

	hs := &http.Server{
		Addr:         fmt.Sprintf(":%d", config.ListenPort),
		Handler:      http.NewServeMux(),
		ReadTimeout:  time.Duration(readTimeout) * time.Second,
		WriteTimeout: time.Duration(writeTimeout) * time.Second,
	}

	server = &HttpServer{
		server:   hs,
		ctx:      ctx,
		cancel:   cancel,
		caFile:   config.CaFile,
		certFile: config.CertFile,
		keyFile:  config.KeyFile,
	}

	return
}

func NewHttpServerFromConfigFile(ctx context.Context, fileName string) (server *HttpServer, err error) {
	var cfg HttpServerConfig
	err = LoadXml(fileName, &cfg)
	if err != nil {
		return nil, err
	}

	return NewHttpServer(ctx, &cfg)
}

func (s *HttpServer) Start() {
	go func() {
		s.server.BaseContext = func(net.Listener) context.Context {
			return s.ctx
		}
		s.server.ConnContext = func(context.Context, net.Conn) context.Context {
			return s.ctx
		}

		var err error
		if s.certFile != "" && s.keyFile != "" {
			if s.caFile != "" {
				pool := x509.NewCertPool()
				caCrt, err := ioutil.ReadFile(s.caFile)
				Panic(err)
				pool.AppendCertsFromPEM(caCrt)
				s.server.TLSConfig = &tls.Config{
					ClientCAs:  pool,
					ClientAuth: tls.RequireAndVerifyClientCert,
				}
			}
			err = s.server.ListenAndServeTLS(s.certFile, s.keyFile)
		} else {
			err = s.server.ListenAndServe()
		}
		Error(err)
	}()

}

func (s *HttpServer) Close() {
	s.cancel()
	s.server.Close()
}

func (s *HttpServer) Handle(pattern string, handler http.Handler) {
	s.server.Handler.(*http.ServeMux).Handle(pattern, handler)
}

func (s *HttpServer) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	s.server.Handler.(*http.ServeMux).HandleFunc(pattern, handler)
}

func ReplyRawText(w http.ResponseWriter, text string) {
	w.Write([]byte(text))
}

func ReplyString(w http.ResponseWriter, res string) {
	s := fmt.Sprintf(`{"result":"%s"}`, res)
	w.Write([]byte(s))
}

func ReplyError(w http.ResponseWriter, res string, err error) {
	s := fmt.Sprintf(`{"result":"%s","detail":"%s"}`, res, err.Error())
	w.Write([]byte(s))
}

func ReplyMap(w http.ResponseWriter, v map[string]interface{}) {
	d, err := json.Marshal(v)
	if err != nil {
		ReplyError(w, "marshal error", err)
		return
	}
	w.Write(d)
}

func Get(url string) ([]byte, error) {
	client := &http.Client{
		Timeout: time.Second * 3,
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func GetJSON(url string) (map[string]interface{}, error) {
	d, err := Get(url)
	if err != nil {
		return nil, err
	}

	var v map[string]interface{}
	err = json.Unmarshal(d, &v)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func GetJSONArray(url string) ([]interface{}, error) {
	d, err := Get(url)
	if err != nil {
		return nil, err
	}

	var v []interface{}
	err = json.Unmarshal(d, &v)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func HttpClientIP(r *http.Request) string {
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	ip := strings.TrimSpace(strings.Split(xForwardedFor, ",")[0])
	if ip != "" {
		return ip
	}

	ip = strings.TrimSpace(r.Header.Get("X-Real-Ip"))
	if ip != "" {
		return ip
	}

	if ip, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return ip
	}

	return ""
}

func Post(url string, form url.Values, headers map[string]string) ([]byte, http.Header, error) {
	req, err := http.NewRequest("POST", url, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if headers != nil {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	client := &http.Client{
		Timeout: time.Second * 3,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	defer resp.Body.Close()

	ce := resp.Header.Get("Content-Encoding")
	if ce == "gzip" {
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, nil, err
		}

		body, err := ioutil.ReadAll(reader)
		if err != nil {
			return nil, nil, err
		}

		return body, resp.Header, nil
	} else {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, err
		}

		return body, resp.Header, nil
	}
}

func PostJson(url string, form url.Values, headers map[string]string) (map[string]interface{}, http.Header, error) {
	d, h, err := Post(url, form, headers)
	if err != nil {
		return nil, nil, err
	}

	var v map[string]interface{}
	err = json.Unmarshal(d, &v)
	if err != nil {
		return nil, nil, err
	}

	return v, h, nil
}

func PostJSONArray(url string, form url.Values, headers map[string]string) ([]interface{}, http.Header, error) {
	d, h, err := Post(url, form, headers)
	if err != nil {
		return nil, nil, err
	}

	var v []interface{}
	err = json.Unmarshal(d, &v)
	if err != nil {
		return nil, nil, err
	}

	return v, h, nil
}
