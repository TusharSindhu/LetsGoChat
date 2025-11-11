// Custom pub/sub message broker using gRPC.
package main

import (
	"context"
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"

	// Import the generated protobuf code.
	pb "github.com/TusharSindhu/LetsGoChat/grpcproto"
)

// brokerServer struct holds the state for the message broker.
type brokerServer struct {
	pb.UnimplementedChatServiceServer
	mu sync.Mutex // Mutex to protect access to the subscribers map.
	// A map to store all connected chat server instances (subscribers).
	subscribers map[pb.ChatService_ReceiveMessagesServer]bool
}

// newBrokerServer creates a new broker server instance.
func newBrokerServer() *brokerServer {
	return &brokerServer{
		subscribers: make(map[pb.ChatService_ReceiveMessagesServer]bool),
	}
}

// BroadcastMessage is the gRPC method called by chat servers to send a message.
func (s *brokerServer) BroadcastMessage(ctx context.Context, msg *pb.ChatMessage) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Iterate over all registered subscribers and send the message to each subscriber's stream.
	for sub := range s.subscribers {
		if err := sub.Send(msg); err != nil {
			log.Printf("could not send message to a subscriber: %v", err)
		}
	}
	// Return an empty response and no error.
	return &pb.Empty{}, nil
}

// ReceiveMessages is the gRPC method that chat servers call to subscribe to messages.
func (s *brokerServer) ReceiveMessages(empty *pb.Empty, stream pb.ChatService_ReceiveMessagesServer) error {
	s.mu.Lock()
	s.subscribers[stream] = true
	s.mu.Unlock()
	log.Println("New chat server subscribed.")

	// Keep the connection open until the client (chat server) disconnects.
	// stream.Context().Done() returns a channel that is closed when the client's context is cancelled.
	<-stream.Context().Done()

	s.mu.Lock()
	delete(s.subscribers, stream)
	s.mu.Unlock()
	log.Println("A chat server unsubscribed.")

	return nil
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Create a new gRPC server.
	s := grpc.NewServer()
	// Register our brokerServer implementation with the gRPC server.
	pb.RegisterChatServiceServer(s, newBrokerServer())

	log.Printf("gRPC broker listening at %v", lis.Addr())
	// Start serving gRPC requests.
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve gRPC: %v", err)
	}
}
