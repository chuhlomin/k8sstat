package main

import (
	"embed"
	"log"
	"net/http"
)

//go:embed index.html
var f embed.FS
var indexData []byte

func init() {
	var err error
	indexData, err = f.ReadFile("index.html")
	if err != nil {
		log.Fatalf("Failed to read index.html file")
	}
}

func handlerIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/html")
	w.Write(indexData)
}
