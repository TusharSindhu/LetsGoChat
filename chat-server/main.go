package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/TusharSindhu/LetsGoChat/grpcproto"
)

// Define a WebSocket upgrader to upgrade HTTP connections to WebSocket connections.
var upgrader = websocket.Upgrader{
	// Allow connections from any origin.
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Define the server struct, representing our chat gateway between WebSocket and gRPC.
type server struct {
	pb.UnimplementedChatServiceServer                          // Embeds the unimplemented gRPC service to satisfy the gRPC interface
	clients                           map[*websocket.Conn]bool // Map of active WebSocket clients
	mu                                sync.Mutex
	grpcClient                        pb.ChatServiceClient // gRPC client used to call broker methods
}

// newServer initializes a new server instance and connects it to the gRPC broker.
func newServer() *server {
	return &server{
		clients: make(map[*websocket.Conn]bool),
	}
}

// serveWs handles websocket requests from the peer.
func (s *server) serveWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer ws.Close()

	s.mu.Lock()
	s.clients[ws] = true
	s.mu.Unlock()
	log.Println("New client connected")

	// Continuously read messages from this WebSocket client.
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			s.mu.Lock()
			delete(s.clients, ws)
			s.mu.Unlock()
			log.Println("Client disconnected:", err)
			break
		}

		_, err = s.grpcClient.BroadcastMessage(context.Background(), &pb.ChatMessage{Body: msg})
		if err != nil {
			log.Printf("could not broadcast message via gRPC: %v", err)
		}
	}
}

// listenForBrokerMessages opens a stream to the broker and pushes messages to clients.
func (s *server) listenForBrokerMessages() {
	stream, err := s.grpcClient.ReceiveMessages(context.Background(), &pb.Empty{})
	if err != nil {
		log.Fatalf("could not open stream to receive messages: %v", err)
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			log.Printf("error receiving message from broker: %v", err)
			time.Sleep(2 * time.Second)
			// Simple reconnect logic
			go s.listenForBrokerMessages()
			return
		}
		// Send the received message to all connected WebSocket clients.
		s.mu.Lock()
		for client := range s.clients {
			err := client.WriteMessage(websocket.TextMessage, msg.Body)
			if err != nil {
				client.Close()
				delete(s.clients, client)
			}
		}
		s.mu.Unlock()
	}
}

// serveHome serves the index.html file.
func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Serves the index.html file from the same directory.
	absPath := "index.html"
	http.ServeFile(w, r, absPath)
}

func main() {
	brokerAddr := os.Getenv("BROKER_ADDR")
	if brokerAddr == "" {
		brokerAddr = "broker:50051" // Default for docker-compose
	}

	// Connect to the gRPC broker (using the variable)
	log.Printf("Connecting to gRPC broker at %s", brokerAddr)
	conn, err := grpc.Dial(brokerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to gRPC broker: %v", err)
	}
	defer conn.Close()

	client := pb.NewChatServiceClient(conn)
	s := newServer()
	s.grpcClient = client

	// Handle the root path "/" to serve the HTML file
	http.HandleFunc("/", serveHome)
	// Handle the WebSocket connection
	http.HandleFunc("/ws", s.serveWs)

	go s.listenForBrokerMessages()

	log.Println("http server started on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
