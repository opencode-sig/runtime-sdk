package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	paymentv1 "github.com/opencode-sig/runtime-sdk/examples/go-template-payment/protobuf/payment/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9004", "payment service gRPC address")
	id := flag.String("id", "pay-1001", "payment id")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	resp, err := paymentv1.NewPaymentServiceClient(conn).GetPayment(ctx, &paymentv1.GetPaymentRequest{Id: *id})
	if err != nil {
		panic(err)
	}
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))
}
