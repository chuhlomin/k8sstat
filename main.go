package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var clientset *kubernetes.Clientset

func main() {
	log.Println("Starting...")

	if err := run(); err != nil {
		log.Printf("ERROR %v", err)
	}

	log.Println("Stopped")
}

func run() error {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	bind := flag.String("bind", "127.0.0.1:8080", "server bind address")
	readTimeout := flag.Duration("read-timeout", 2*time.Second, "server read timeout")

	flag.Parse()

	log.Printf("Creating K8s client...")
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return errors.Wrap(err, "build config")
	}

	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		return errors.Wrap(err, "create client set from config")
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5, "application/json"))

	r.Get("/health", handlerHealth)
	r.Get("/stats", handlerStats)

	srv := &http.Server{
		Addr:        *bind,
		ReadTimeout: *readTimeout,
		Handler:     r,
	}

	timeoutContext, doCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer doCancel()
	shutdownContext, doShutdown := context.WithCancel(timeoutContext)

	go listenForSignals(shutdownContext, doShutdown, srv)

	log.Printf("HTTP server listening on: %v", *bind)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("HTTP server ListenAndServe: %v", err)
	}

	<-shutdownContext.Done()

	return nil
}

func listenForSignals(ctx context.Context, doShutdown context.CancelFunc, srv *http.Server) {
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt, os.Kill)
	<-sigint

	log.Println("Shutting down...")
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP server Shutdown: %v", err)
	}
	doShutdown()
}
