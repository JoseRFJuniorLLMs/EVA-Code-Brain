package main

import (
	"codebrain/graph"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, assuming environment variables are set.")
	}

	uri := os.Getenv("NEO4J_URI")
	user := os.Getenv("NEO4J_USER")
	password := os.Getenv("NEO4J_PASSWORD")

	if uri == "" || user == "" || password == "" {
		log.Fatal("NEO4J_URI, NEO4J_USER, and NEO4J_PASSWORD must be set")
	}

	fmt.Printf("Connecting to Neo4j at %s as %s...\n", uri, user)

	config := &graph.Config{
		URI:                uri,
		User:               user,
		Password:           password,
		MaxPoolSize:        10,
		AcquisitionTimeout: 60 * time.Second,
	}

	client, err := graph.NewNeo4jClient(config)
	if err != nil {
		log.Fatalf("Failed to create Neo4j client: %v", err)
	}
	defer client.Close(context.Background()) // Note: Close signature in client needs context? No, driver.Close usually takes ctx or nothing. Checking client code...

	// Correcting Close call based on implementation
	// func (c *Neo4jClient) Close(ctx context.Context) error

	ctx := context.Background()

	// Create a test node
	fmt.Println("Creating test node...")
	query := `
		MERGE (p:Pessoa {nome: "Jose"})
		MERGE (e:Emoção {tipo: "Curiosidade"})
		MERGE (p)-[:SENTE]->(e)
		RETURN p, e
	`

	err = client.ExecuteWrite(ctx, query, nil)
	if err != nil {
		log.Fatalf("Failed to execute write query: %v", err)
	}

	fmt.Println("✅ Success! Created (:Pessoa {nome: 'Jose'})-[:SENTE]->(:Emoção {tipo: 'Curiosidade'})")
}
