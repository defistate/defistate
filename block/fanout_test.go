package block

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLogger is a simple thread-safe logger for testing purposes.
type mockLogger struct {
	mu    sync.Mutex
	warns []string
	infos []string
}

func newMockLogger() *mockLogger {
	return &mockLogger{}
}

func (m *mockLogger) Info(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.infos = append(m.infos, msg)
}

func (m *mockLogger) Warn(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Simple formatting to capture key-value pairs for assertion
	logLine := msg
	if len(args) > 0 {
		for i := 0; i < len(args); i += 2 {
			logLine = fmt.Sprintf("%s %v=%v", logLine, args[i], args[i+1])
		}
	}
	m.warns = append(m.warns, logLine)
}
func (m *mockLogger) Debug(msg string, args ...any) {}
func (m *mockLogger) Error(msg string, args ...any) {}

func (m *mockLogger) WarnCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.warns)
}

func (m *mockLogger) HasWarningWithMessage(substr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.warns {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// Helper function to create a mock block for testing.
func newTestBlock(num int) *types.Block {
	return types.NewBlockWithHeader(&types.Header{
		Number: big.NewInt(int64(num)),
	})
}

// TestFanOutBlock_ConfigValidation tests all invalid configuration scenarios.
func TestFanOutBlock_ConfigValidation(t *testing.T) {
	t.Parallel()

	// A valid config to use as a base
	baseInput := make(chan *types.Block)
	baseOutputs := []chan *types.Block{make(chan *types.Block)}
	baseTags := []string{"a"}
	baseLogger := newMockLogger()

	testCases := []struct {
		name        string
		cfg         FanOutBlockConfig
		expectedErr error
	}{
		{
			name:        "nil input channel",
			cfg:         FanOutBlockConfig{Outputs: baseOutputs, OutputTags: baseTags, Logger: baseLogger},
			expectedErr: errors.New("config: input channel cannot be nil"),
		},
		{
			name:        "zero output channels",
			cfg:         FanOutBlockConfig{Input: baseInput, Logger: baseLogger},
			expectedErr: errors.New("config: must have at least one output channel"),
		},
		{
			name:        "mismatched outputs and tags length",
			cfg:         FanOutBlockConfig{Input: baseInput, Outputs: baseOutputs, OutputTags: []string{"a", "b"}, Logger: baseLogger},
			expectedErr: errors.New("config: outputs and outputTags must have the same length"),
		},
		{
			name:        "nil logger",
			cfg:         FanOutBlockConfig{Input: baseInput, Outputs: baseOutputs, OutputTags: baseTags},
			expectedErr: errors.New("config: logger cannot be nil"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := FanOutBlock(context.Background(), tc.cfg)
			require.Error(t, err)
			assert.Equal(t, tc.expectedErr.Error(), err.Error())
		})
	}
}

// TestFanOutBlock_Success tests the happy path where all consumers are fast
// and receive all blocks in order.
func TestFanOutBlock_Success(t *testing.T) {
	t.Parallel()

	input := make(chan *types.Block)
	blockCount := 20
	outputs := []chan *types.Block{
		make(chan *types.Block, blockCount),
		make(chan *types.Block, blockCount),
		make(chan *types.Block, blockCount),
	}
	outputTags := []string{"c1", "c2", "c3"}

	cfg := FanOutBlockConfig{
		Input:      input,
		Outputs:    outputs,
		OutputTags: outputTags,
		Logger:     newMockLogger(),
	}

	err := FanOutBlock(context.Background(), cfg)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(len(outputs))

	// Start consumers
	for i := 0; i < len(outputs); i++ {
		go func(consumerIndex int, ch <-chan *types.Block) {
			defer wg.Done()
			blockNum := 0
			for block := range ch {
				assert.Equal(t, int64(blockNum), block.Number().Int64(), "Consumer %d received block out of order", consumerIndex)
				blockNum++
			}
			assert.Equal(t, blockCount, blockNum, "Consumer %d did not receive all blocks", consumerIndex)
		}(i, outputs[i])
	}

	// Start producer
	for i := 0; i < blockCount; i++ {
		input <- newTestBlock(i)
	}
	close(input)

	wg.Wait()
}

// TestFanOutBlock_NonBlockingSendAndSkip verifies that a slow consumer's
// channel being full does not block other consumers, and that a warning is logged.
func TestFanOutBlock_NonBlockingSendAndSkip(t *testing.T) {
	t.Parallel()

	input := make(chan *types.Block)
	fastConsumerChan := make(chan *types.Block, 10)
	slowConsumerChan := make(chan *types.Block, 1) // Tiny buffer to force it to block
	mockLogger := newMockLogger()

	cfg := FanOutBlockConfig{
		Input:      input,
		Outputs:    []chan *types.Block{fastConsumerChan, slowConsumerChan},
		OutputTags: []string{"fast", "slow"},
		Logger:     mockLogger,
	}

	err := FanOutBlock(context.Background(), cfg)
	require.NoError(t, err)

	// Send two blocks. The slow consumer's buffer will fill after the first.
	input <- newTestBlock(0) // Both consumers receive this.
	input <- newTestBlock(1) // Fast consumer receives, slow one is skipped.
	input <- newTestBlock(2) // Fast consumer receives, slow one is skipped again.
	close(input)

	// Verify the fast consumer got all blocks
	receivedBlocksFast := []*types.Block{}
	for block := range fastConsumerChan {
		receivedBlocksFast = append(receivedBlocksFast, block)
	}
	require.Len(t, receivedBlocksFast, 3)
	assert.Equal(t, int64(0), receivedBlocksFast[0].Number().Int64())
	assert.Equal(t, int64(1), receivedBlocksFast[1].Number().Int64())
	assert.Equal(t, int64(2), receivedBlocksFast[2].Number().Int64())

	// Verify the slow consumer only got the first block
	receivedBlocksSlow := []*types.Block{}
	// Drain the channel non-blockingly
	for len(slowConsumerChan) > 0 {
		receivedBlocksSlow = append(receivedBlocksSlow, <-slowConsumerChan)
	}
	require.Len(t, receivedBlocksSlow, 1)
	assert.Equal(t, int64(0), receivedBlocksSlow[0].Number().Int64())

	// Verify that warnings were logged for the slow consumer
	assert.Equal(t, 2, mockLogger.WarnCount(), "Should have logged two warnings for the blocked consumer")
	assert.True(t, mockLogger.HasWarningWithMessage("output channel blocked, skipping send tag=slow"))
}

// TestFanOutBlock_ContextCancellation tests that canceling the context
// correctly shuts down the fan-out and closes all output channels.
func TestFanOutBlock_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	input := make(chan *types.Block)
	outputs := []chan *types.Block{make(chan *types.Block), make(chan *types.Block)}

	cfg := FanOutBlockConfig{
		Input:      input,
		Outputs:    outputs,
		OutputTags: []string{"a", "b"},
		Logger:     newMockLogger(),
	}
	err := FanOutBlock(ctx, cfg)
	require.NoError(t, err)

	// Send one block to ensure the goroutine is running
	input <- newTestBlock(1)

	// Cancel the context
	cancel()

	// All output channels should be closed due to cancellation.
	for _, ch := range outputs {
		select {
		case _, ok := <-ch:
			// We might or might not have received the first block.
			// The important part is that the channel becomes closed.
			if ok {
				_, okAfterRead := <-ch
				assert.False(t, okAfterRead, "Channel should be closed after cancellation")
			} else {
				assert.False(t, ok, "Channel should be closed after cancellation")
			}
		case <-time.After(1 * time.Second):
			t.Fatal("Channel was not closed after context cancellation")
		}
	}
}
