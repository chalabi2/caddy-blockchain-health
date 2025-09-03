package blockchain_health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// CosmosHandler handles health checks for Cosmos-based blockchain nodes
type CosmosHandler struct {
	client *http.Client
	logger *zap.Logger
}

// NewCosmosHandler creates a new Cosmos protocol handler
func NewCosmosHandler(timeout time.Duration, logger *zap.Logger) *CosmosHandler {
	return &CosmosHandler{
		client: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// CosmosStatus represents the response from Cosmos /status endpoint
type CosmosStatus struct {
	Result struct {
		SyncInfo struct {
			LatestBlockHeight string `json:"latest_block_height"`
			CatchingUp        bool   `json:"catching_up"`
		} `json:"sync_info"`
	} `json:"result"`
}

// CosmosRESTSyncing represents the response from Cosmos REST /cosmos/base/tendermint/v1beta1/syncing
type CosmosRESTSyncing struct {
	Syncing bool `json:"syncing"`
}

// CosmosRESTLatestBlock represents the response from Cosmos REST latest block endpoint
type CosmosRESTLatestBlock struct {
	Block struct {
		Header struct {
			Height string `json:"height"`
		} `json:"header"`
	} `json:"block"`
}

// CheckHealth implements ProtocolHandler for Cosmos nodes
func (c *CosmosHandler) CheckHealth(ctx context.Context, node NodeConfig) (*NodeHealth, error) {
	start := time.Now()
	health := &NodeHealth{
		Name:      node.Name,
		URL:       node.URL,
		Healthy:   false,
		LastCheck: time.Now(),
	}

	c.logger.Debug("starting Cosmos health check",
		zap.String("node", node.Name),
		zap.String("url", node.URL),
		zap.String("type", string(node.Type)))

	var blockHeight uint64
	var catchingUp bool
	var err error

	// Check if this is a REST API node or RPC node
	if node.Metadata["service_type"] == "api" {
		// This is a REST API node - use REST directly
		c.logger.Debug("using REST API for API node",
			zap.String("node", node.Name),
			zap.String("url", node.URL))
		blockHeight, catchingUp, err = c.checkRESTStatus(ctx, node.URL)
	} else {
		// This is an RPC node - try RPC first, fallback to REST if available
		c.logger.Debug("using RPC for RPC node",
			zap.String("node", node.Name),
			zap.String("url", node.URL))
		blockHeight, catchingUp, err = c.checkRPCStatus(ctx, node.URL)
		if err != nil {
			c.logger.Debug("RPC check failed, trying REST API fallback",
				zap.String("node", node.Name),
				zap.String("url", node.URL),
				zap.Error(err))

			// If RPC fails and we have an API URL, try REST
			if node.APIURL != "" {
				blockHeight, catchingUp, err = c.checkRESTStatus(ctx, node.APIURL)
			}
		}
	}

	if err != nil {
		c.logger.Warn("all health checks failed for node",
			zap.String("node", node.Name),
			zap.String("url", node.URL),
			zap.Error(err))
		health.LastError = err.Error()
		health.ResponseTime = time.Since(start)
		return health, nil // Don't return error, just mark as unhealthy
	}

	c.logger.Debug("health check successful",
		zap.String("node", node.Name),
		zap.Uint64("block_height", blockHeight),
		zap.Bool("catching_up", catchingUp))

	// Additionally check WebSocket if configured
	if node.WebSocketURL != "" {
		wsHealthy := c.checkWebSocketHealth(ctx, node.WebSocketURL)
		if !wsHealthy {
			c.logger.Debug("WebSocket health check failed",
				zap.String("node", node.Name),
				zap.String("websocket_url", node.WebSocketURL))
			// WebSocket failure doesn't make the node unhealthy if HTTP works
			// but we log it for monitoring
		}
	}

	health.BlockHeight = blockHeight
	health.CatchingUp = &catchingUp
	health.ResponseTime = time.Since(start)

	// Node is healthy if we got a response and it's not catching up
	health.Healthy = !catchingUp

	c.logger.Debug("health check completed",
		zap.String("node", node.Name),
		zap.Bool("healthy", health.Healthy),
		zap.String("error", health.LastError))

	return health, nil
}

// GetBlockHeight implements ProtocolHandler for Cosmos nodes
func (c *CosmosHandler) GetBlockHeight(ctx context.Context, url string) (uint64, error) {
	// Try RPC first
	height, _, err := c.checkRPCStatus(ctx, url)
	if err != nil {
		// If this looks like a REST URL, try REST instead
		// Note: This fallback should rarely be used - prefer explicit service type configuration
		if strings.Contains(url, "/cosmos/") {
			height, _, err = c.checkRESTStatus(ctx, url)
		}
	}
	return height, err
}

// checkRPCStatus checks Cosmos node status via RPC endpoint
func (c *CosmosHandler) checkRPCStatus(ctx context.Context, url string) (uint64, bool, error) {
	statusURL := fmt.Sprintf("%s/status", strings.TrimSuffix(url, "/"))

	c.logger.Debug("checking RPC status",
		zap.String("status_url", statusURL))

	req, err := http.NewRequestWithContext(ctx, "GET", statusURL, nil)
	if err != nil {
		return 0, false, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Debug("RPC request failed",
			zap.String("url", statusURL),
			zap.Error(err))
		return 0, false, fmt.Errorf("RPC request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("Failed to close response body", zap.Error(err))
		}
	}()

	c.logger.Debug("RPC response received",
		zap.String("url", statusURL),
		zap.Int("status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("RPC status %d", resp.StatusCode)
	}

	var status CosmosStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		c.logger.Debug("failed to decode RPC response",
			zap.String("url", statusURL),
			zap.Error(err))
		return 0, false, fmt.Errorf("decoding RPC response: %w", err)
	}

	c.logger.Debug("RPC response decoded",
		zap.String("url", statusURL),
		zap.String("block_height", status.Result.SyncInfo.LatestBlockHeight),
		zap.Bool("catching_up", status.Result.SyncInfo.CatchingUp))

	height, err := strconv.ParseUint(status.Result.SyncInfo.LatestBlockHeight, 10, 64)
	if err != nil {
		c.logger.Debug("failed to parse block height",
			zap.String("url", statusURL),
			zap.String("height_string", status.Result.SyncInfo.LatestBlockHeight),
			zap.Error(err))
		return 0, false, fmt.Errorf("parsing block height: %w", err)
	}

	return height, status.Result.SyncInfo.CatchingUp, nil
}

// checkRESTStatus checks Cosmos node status via REST API
func (c *CosmosHandler) checkRESTStatus(ctx context.Context, baseURL string) (uint64, bool, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Check syncing status
	syncingURL := fmt.Sprintf("%s/cosmos/base/tendermint/v1beta1/syncing", baseURL)

	c.logger.Debug("checking REST syncing status",
		zap.String("syncing_url", syncingURL))

	req, err := http.NewRequestWithContext(ctx, "GET", syncingURL, nil)
	if err != nil {
		return 0, false, fmt.Errorf("creating syncing request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Debug("REST syncing request failed",
			zap.String("url", syncingURL),
			zap.Error(err))
		return 0, false, fmt.Errorf("REST syncing request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("Failed to close response body", zap.Error(err))
		}
	}()

	c.logger.Debug("REST syncing response received",
		zap.String("url", syncingURL),
		zap.Int("status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("REST syncing status %d", resp.StatusCode)
	}

	var syncStatus CosmosRESTSyncing
	if err := json.NewDecoder(resp.Body).Decode(&syncStatus); err != nil {
		c.logger.Debug("failed to decode REST syncing response",
			zap.String("url", syncingURL),
			zap.Error(err))
		return 0, false, fmt.Errorf("decoding REST syncing response: %w", err)
	}

	c.logger.Debug("REST syncing response decoded",
		zap.String("url", syncingURL),
		zap.Bool("syncing", syncStatus.Syncing))

	// Get latest block height
	blockURL := fmt.Sprintf("%s/cosmos/base/tendermint/v1beta1/blocks/latest", baseURL)

	c.logger.Debug("checking REST latest block",
		zap.String("block_url", blockURL))

	req, err = http.NewRequestWithContext(ctx, "GET", blockURL, nil)
	if err != nil {
		return 0, false, fmt.Errorf("creating block request: %w", err)
	}

	resp, err = c.client.Do(req)
	if err != nil {
		c.logger.Debug("REST block request failed",
			zap.String("url", blockURL),
			zap.Error(err))
		return 0, false, fmt.Errorf("REST block status %d", resp.StatusCode)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("Failed to close response body", zap.Error(err))
		}
	}()

	c.logger.Debug("REST block response received",
		zap.String("url", blockURL),
		zap.Int("status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("REST block status %d", resp.StatusCode)
	}

	var blockResp CosmosRESTLatestBlock
	if err := json.NewDecoder(resp.Body).Decode(&blockResp); err != nil {
		c.logger.Debug("failed to decode REST block response",
			zap.String("url", blockURL),
			zap.Error(err))
		return 0, false, fmt.Errorf("decoding REST block response: %w", err)
	}

	c.logger.Debug("REST block response decoded",
		zap.String("url", blockURL),
		zap.String("height", blockResp.Block.Header.Height))

	height, err := strconv.ParseUint(blockResp.Block.Header.Height, 10, 64)
	if err != nil {
		c.logger.Debug("failed to parse REST block height",
			zap.String("url", blockURL),
			zap.String("height_string", blockResp.Block.Header.Height),
			zap.Error(err))
		return 0, false, fmt.Errorf("parsing REST block height: %w", err)
	}

	// For REST API, syncing = catching up
	return height, syncStatus.Syncing, nil
}

// checkWebSocketHealth tests WebSocket connectivity for Cosmos nodes
func (c *CosmosHandler) checkWebSocketHealth(ctx context.Context, wsURL string) bool {
	// Parse and validate WebSocket URL
	u, err := url.Parse(wsURL)
	if err != nil {
		c.logger.Debug("Invalid WebSocket URL", zap.String("url", wsURL), zap.Error(err))
		return false
	}

	// Convert http/https to ws/wss
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// Already correct
	default:
		c.logger.Debug("Unsupported WebSocket scheme", zap.String("scheme", u.Scheme))
		return false
	}

	// Create dialer with timeout
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	// Attempt WebSocket connection
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		c.logger.Debug("WebSocket connection failed", zap.String("url", u.String()), zap.Error(err))
		return false
	}
	defer func() {
		if err := conn.Close(); err != nil {
			c.logger.Debug("Failed to close connection", zap.Error(err))
		}
	}()

	// Test with a simple Cosmos WebSocket subscription
	testMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "subscribe",
		"id":      1,
		"params": map[string]interface{}{
			"query": "tm.event = 'NewBlock'",
		},
	}

	// Send test message
	if err := conn.WriteJSON(testMsg); err != nil {
		c.logger.Debug("WebSocket write failed", zap.Error(err))
		return false
	}

	// Set read deadline for response
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		c.logger.Debug("Failed to set read deadline", zap.Error(err))
		return false
	}

	// Try to read response
	var response map[string]interface{}
	if err := conn.ReadJSON(&response); err != nil {
		c.logger.Debug("WebSocket read failed", zap.Error(err))
		return false
	}

	c.logger.Debug("WebSocket health check successful", zap.String("url", u.String()))
	return true
}

// EVMHandler handles health checks for EVM-based blockchain nodes
type EVMHandler struct {
	client *http.Client
	logger *zap.Logger
}

// NewEVMHandler creates a new EVM protocol handler
func NewEVMHandler(timeout time.Duration, logger *zap.Logger) *EVMHandler {
	return &EVMHandler{
		client: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// EVMJSONRPCRequest represents a JSON-RPC request
type EVMJSONRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// EVMJSONRPCResponse represents a JSON-RPC response
type EVMJSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	ID int `json:"id"`
}

// CheckHealth implements ProtocolHandler for EVM nodes
func (e *EVMHandler) CheckHealth(ctx context.Context, node NodeConfig) (*NodeHealth, error) {
	start := time.Now()
	health := &NodeHealth{
		Name:      node.Name,
		URL:       node.URL,
		Healthy:   false,
		LastCheck: time.Now(),
	}

	blockHeight, err := e.GetBlockHeight(ctx, node.URL)
	if err != nil {
		health.LastError = err.Error()
		health.ResponseTime = time.Since(start)
		return health, nil // Don't return error, just mark as unhealthy
	}

	health.BlockHeight = blockHeight
	health.ResponseTime = time.Since(start)
	health.Healthy = true
	// EVM nodes don't have a "catching up" concept like Cosmos
	// If we can get a block height, we consider the node healthy

	// Additionally check WebSocket if configured
	if node.WebSocketURL != "" {
		wsHealthy := e.checkWebSocketHealth(ctx, node.WebSocketURL)
		if !wsHealthy {
			e.logger.Debug("WebSocket health check failed",
				zap.String("node", node.Name),
				zap.String("websocket_url", node.WebSocketURL))
			// WebSocket failure doesn't make the node unhealthy if HTTP works
			// but we log it for monitoring
		}
	}

	return health, nil
}

// GetBlockHeight implements ProtocolHandler for EVM nodes
func (e *EVMHandler) GetBlockHeight(ctx context.Context, url string) (uint64, error) {
	reqBody := EVMJSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "eth_blockNumber",
		Params:  []interface{}{},
		ID:      1,
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(reqBytes)))
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("JSON-RPC request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			e.logger.Debug("Failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("JSON-RPC status %d", resp.StatusCode)
	}

	var rpcResp EVMJSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return 0, fmt.Errorf("decoding JSON-RPC response: %w", err)
	}

	if rpcResp.Error != nil {
		return 0, fmt.Errorf("JSON-RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	heightStr, ok := rpcResp.Result.(string)
	if !ok {
		return 0, fmt.Errorf("invalid block height response type")
	}

	// Remove 0x prefix if present
	heightStr = strings.TrimPrefix(heightStr, "0x")

	height, err := strconv.ParseUint(heightStr, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing block height: %w", err)
	}

	return height, nil
}

// checkWebSocketHealth tests WebSocket connectivity for EVM nodes
func (e *EVMHandler) checkWebSocketHealth(ctx context.Context, wsURL string) bool {
	// Parse and validate WebSocket URL
	u, err := url.Parse(wsURL)
	if err != nil {
		e.logger.Debug("Invalid WebSocket URL", zap.String("url", wsURL), zap.Error(err))
		return false
	}

	// Convert http/https to ws/wss
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// Already correct
	default:
		e.logger.Debug("Unsupported WebSocket scheme", zap.String("scheme", u.Scheme))
		return false
	}

	// Create dialer with timeout
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	// Attempt WebSocket connection
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		e.logger.Debug("WebSocket connection failed", zap.String("url", u.String()), zap.Error(err))
		return false
	}
	defer func() {
		if err := conn.Close(); err != nil {
			e.logger.Debug("Failed to close connection", zap.Error(err))
		}
	}()

	// Test with a simple EVM WebSocket subscription (newHeads)
	testMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_subscribe",
		"params":  []interface{}{"newHeads"},
		"id":      1,
	}

	// Send test message
	if err := conn.WriteJSON(testMsg); err != nil {
		e.logger.Debug("WebSocket write failed", zap.Error(err))
		return false
	}

	// Set read deadline for response
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		e.logger.Debug("Failed to set read deadline", zap.Error(err))
		return false
	}

	// Try to read response
	var response map[string]interface{}
	if err := conn.ReadJSON(&response); err != nil {
		e.logger.Debug("WebSocket read failed", zap.Error(err))
		return false
	}

	e.logger.Debug("WebSocket health check successful", zap.String("url", u.String()))
	return true
}
