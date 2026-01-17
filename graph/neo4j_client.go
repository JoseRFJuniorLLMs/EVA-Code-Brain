package graph

import (
	"context"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Neo4jClient struct {
	driver neo4j.DriverWithContext
	config *Config
}

type Config struct {
	URI                string
	User               string
	Password           string
	MaxPoolSize        int
	AcquisitionTimeout time.Duration
}

func NewNeo4jClient(config *Config) (*Neo4jClient, error) {
	driver, err := neo4j.NewDriverWithContext(
		config.URI,
		neo4j.BasicAuth(config.User, config.Password, ""),
		func(c *neo4j.Config) {
			c.MaxConnectionPoolSize = config.MaxPoolSize
			c.ConnectionAcquisitionTimeout = config.AcquisitionTimeout
		},
	)

	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return nil, err
	}

	return &Neo4jClient{driver: driver, config: config}, nil
}

func (c *Neo4jClient) Close(ctx context.Context) error {
	return c.driver.Close(ctx)
}

func (c *Neo4jClient) ExecuteRead(
	ctx context.Context,
	query string,
	params map[string]interface{},
) ([]map[string]interface{}, error) {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeRead,
	})
	defer session.Close(ctx)

	result, err := session.Run(ctx, query, params)
	if err != nil {
		return nil, err
	}

	var records []map[string]interface{}
	for result.Next(ctx) {
		record := result.Record()
		recordMap := make(map[string]interface{})

		for _, key := range record.Keys {
			value, _ := record.Get(key)
			recordMap[key] = value
		}

		records = append(records, recordMap)
	}

	return records, result.Err()
}

func (c *Neo4jClient) ExecuteWrite(
	ctx context.Context,
	query string,
	params map[string]interface{},
) error {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeWrite,
	})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		return result.Consume(ctx)
	})

	return err
}
