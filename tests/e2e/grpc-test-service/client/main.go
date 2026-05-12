package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	pb "grpc-test-service/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	addr := flag.String("addr", "", "server address (host:port)")
	test := flag.String("test", "both", "test to run: unary, stream, or both")
	wait := flag.Bool("wait", false, "block forever (used for long-running client pod)")
	flag.Parse()

	if *wait {
		fmt.Println("grpc-client ready")
		// Block forever without triggering the Go deadlock detector.
		// select{} with no cases causes a runtime panic ("all goroutines are
		// asleep - deadlock!") which crashes the container; an infinite sleep
		// loop avoids that.
		for {
			time.Sleep(24 * time.Hour)
		}
	}

	if *addr == "" {
		fmt.Fprintln(os.Stderr, "error: --addr is required")
		os.Exit(1)
	}

	conn, err := grpc.NewClient(*addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: dial: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := pb.NewGreeterClient(conn)

	switch *test {
	case "unary":
		if err := runUnary(client); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: unary: %v\n", err)
			os.Exit(1)
		}
	case "stream":
		if err := runStream(client); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: streaming: %v\n", err)
			os.Exit(1)
		}
	case "both":
		if err := runUnary(client); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: unary: %v\n", err)
			os.Exit(1)
		}
		if err := runStream(client); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: streaming: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "error: unknown test %q\n", *test)
		os.Exit(1)
	}
}

func runUnary(client pb.GreeterClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	resp, err := client.SayHello(ctx, &pb.HelloRequest{Name: "e2e-test"})
	if err != nil {
		return fmt.Errorf("SayHello RPC failed: %w", err)
	}
	if resp.Message != "Hello e2e-test" {
		return fmt.Errorf("unexpected response: got %q, want %q", resp.Message, "Hello e2e-test")
	}
	fmt.Println("PASS: unary RPC")
	return nil
}

func runStream(client pb.GreeterClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	stream, err := client.SayHelloStream(ctx, &pb.HelloRequest{Name: "e2e-test"})
	if err != nil {
		return fmt.Errorf("SayHelloStream RPC failed: %w", err)
	}

	var count int
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("stream.Recv failed: %w", err)
		}
		count++
	}

	if count != 5 {
		return fmt.Errorf("expected 5 messages, got %d", count)
	}
	fmt.Println("PASS: streaming RPC (5 messages)")
	return nil
}
