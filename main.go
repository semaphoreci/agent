package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type Server struct {
	Host  string
	Port  int
	State string
}

func (s *Server) Serve() {
	r := mux.NewRouter().StrictSlash(true)
	address := fmt.Sprintf("%s:%d", s.Host, s.Port)

	r.HandleFunc("/status", s.Status).Methods("GET")
	r.HandleFunc("/run", s.Run).Methods("POST")
	r.HandleFunc("/stop", s.Stop).Methods("POST")

	loggedRouter := handlers.LoggingHandler(os.Stdout, r)

	log.Fatal(http.ListenAndServe(address, loggedRouter))
}

func (s *Server) Status(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(400)
	fmt.Fprintf(w, `{"state": "%s", "uptime": "pretty long time"}`, s.State)
}

func (s *Server) Run(w http.ResponseWriter, r *http.Request) {
	s.State = "received-job"

	jobRequest := JobRequest{}

	err := json.NewDecoder(r.Body).Decode(&jobRequest)

	if err != nil {
		fmt.Fprintf(w, `{"message": "%s"}`, err)
		return
	}

	job := Job{Request: jobRequest}

	s.State = "job-started"

	go job.Run()
}

func (s *Server) Stop(w http.ResponseWriter, r *http.Request) {
	s.unsuported(w)
}

func (s *Server) unsuported(w http.ResponseWriter) {
	w.WriteHeader(400)
	fmt.Fprintf(w, `{"message": "not supported"}`)
}

func main() {
	action := os.Args[1]

	switch action {
	case "serve":
		server := Server{Host: "0.0.0.0", Port: 8000, State: "waiting for job"}
		server.Serve()

	case "run":
		job, err := NewJobFromYaml(os.Args[2])

		if err != nil {
			panic(err)
		}

		job.Run()

	case "version":
		fmt.Println("v0.0.1")
	}
}
