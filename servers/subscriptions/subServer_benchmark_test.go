package subscriptions

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultSubscriptionBenchmarkPayloadBytes = 256
	defaultSubscriptionBenchmarkParallelism  = 1
	defaultSubscriptionBenchmarkKeysPerUser  = 1
)

type subscriptionBenchmarkScenario struct {
	clients       int
	keys          int
	keysPerClient int
	parallelism   int
	payloadBytes  int
}

func (s subscriptionBenchmarkScenario) Name() string {
	return fmt.Sprintf(
		"clients=%d/keys=%d/keys_per_client=%d/parallel=%d/payload=%dB",
		s.clients,
		s.keys,
		s.keysPerClient,
		s.parallelism,
		s.payloadBytes,
	)
}

// BenchmarkSubscriptionServer can be scaled with:
// TSU_SUB_BENCH_CLIENTS, TSU_SUB_BENCH_KEYS, TSU_SUB_BENCH_KEYS_PER_CLIENT,
// TSU_SUB_BENCH_PARALLELISM and TSU_SUB_BENCH_PAYLOAD_BYTES.
func BenchmarkSubscriptionServer(b *testing.B) {
	for _, scenario := range subscriptionBenchmarkScenarios(b) {
		scenario := scenario
		b.Run(scenario.Name(), func(b *testing.B) {
			benchmarkSubscriptionServerScenario(b, scenario)
		})
	}
}

func benchmarkSubscriptionServerScenario(b *testing.B, scenario subscriptionBenchmarkScenario) {
	b.Helper()

	if scenario.clients <= 0 {
		b.Fatalf("clients must be > 0, got %d", scenario.clients)
	}
	if scenario.keys <= 0 {
		b.Fatalf("keys must be > 0, got %d", scenario.keys)
	}
	if scenario.keysPerClient <= 0 || scenario.keysPerClient > scenario.keys {
		b.Fatalf("keysPerClient must be in [1,%d], got %d", scenario.keys, scenario.keysPerClient)
	}
	if scenario.parallelism <= 0 {
		b.Fatalf("parallelism must be > 0, got %d", scenario.parallelism)
	}
	if scenario.payloadBytes < 0 {
		b.Fatalf("payloadBytes must be >= 0, got %d", scenario.payloadBytes)
	}

	resetSubscriptionsForTests(b)

	server := httptest.NewServer(http.HandlerFunc(HandleWS))
	defer server.Close()

	keys := subscriptionBenchmarkKeys(scenario.keys)
	clients, keySubscribers, received, readers, readErrs := openSubscriptionBenchmarkClients(b, server.URL, keys, scenario.keysPerClient, scenario.clients)

	payload := bytes.Repeat([]byte("x"), scenario.payloadBytes)
	stats := StatsSnapshot()
	expectedSubscriptions := scenario.clients * scenario.keysPerClient
	expectedActiveKeys := scenario.keys
	if expectedSubscriptions < expectedActiveKeys {
		expectedActiveKeys = expectedSubscriptions
	}

	if stats.ActiveClients != scenario.clients {
		b.Fatalf("active clients = %d, want %d", stats.ActiveClients, scenario.clients)
	}
	if stats.KeysWithSubscribers != expectedActiveKeys {
		b.Fatalf("keys with subscribers = %d, want %d", stats.KeysWithSubscribers, expectedActiveKeys)
	}
	if stats.ActiveSubscriptions != expectedSubscriptions {
		b.Fatalf("active subscriptions = %d, want %d", stats.ActiveSubscriptions, expectedSubscriptions)
	}
	if stats.PendingAuthKeys != 0 {
		b.Fatalf("pending auth keys = %d, want 0", stats.PendingAuthKeys)
	}

	b.ReportAllocs()
	b.ReportMetric(float64(scenario.clients), "clients")
	b.ReportMetric(float64(scenario.keys), "keys")
	b.ReportMetric(float64(expectedActiveKeys), "active_keys")
	b.ReportMetric(float64(scenario.keysPerClient), "keys/client")
	b.ReportMetric(float64(expectedSubscriptions), "subscriptions")
	b.ReportMetric(float64(expectedSubscriptions)/float64(expectedActiveKeys), "subs/key")
	b.ReportMetric(float64(scenario.payloadBytes), "payload_B")

	var sequence atomic.Uint64
	b.SetParallelism(scenario.parallelism)
	b.ResetTimer()
	start := time.Now()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := int(sequence.Add(1)-1) % len(keys)
			NotifySubscribers(keys[idx], payload)
		}
	})
	elapsed := time.Since(start)
	b.StopTimer()

	totalWrites := int(sequence.Load())
	totalNotifications := subscriptionBenchmarkNotificationCount(totalWrites, keySubscribers)

	if elapsed > 0 {
		b.ReportMetric(float64(totalWrites)/elapsed.Seconds(), "writes/s")
		b.ReportMetric(float64(totalNotifications)/elapsed.Seconds(), "notifications/s")
	}
	if totalWrites > 0 {
		b.ReportMetric(float64(totalNotifications)/float64(totalWrites), "notifs/write")
	}

	if err := waitForSubscriptionBenchmarkNotifications(totalNotifications, received, readErrs, 5*time.Second); err != nil {
		closeSubscriptionBenchmarkClients(clients)
		readers.Wait()
		b.Fatal(err)
	}

	closeSubscriptionBenchmarkClients(clients)
	readers.Wait()

	select {
	case err := <-readErrs:
		b.Fatalf("websocket reader error: %v", err)
	default:
	}
}

func subscriptionBenchmarkScenarios(b *testing.B) []subscriptionBenchmarkScenario {
	b.Helper()

	clientCounts := benchmarkIntListEnv(b, "TSU_SUB_BENCH_CLIENTS", []int{10, 100})
	keyCounts := benchmarkIntListEnv(b, "TSU_SUB_BENCH_KEYS", []int{1, 10})
	keysPerClient := benchmarkIntEnv(b, "TSU_SUB_BENCH_KEYS_PER_CLIENT", defaultSubscriptionBenchmarkKeysPerUser)
	parallelism := benchmarkIntEnv(b, "TSU_SUB_BENCH_PARALLELISM", defaultSubscriptionBenchmarkParallelism)
	payloadBytes := benchmarkIntEnv(b, "TSU_SUB_BENCH_PAYLOAD_BYTES", defaultSubscriptionBenchmarkPayloadBytes)

	scenarios := make([]subscriptionBenchmarkScenario, 0, len(clientCounts)*len(keyCounts))
	for _, clients := range clientCounts {
		for _, keys := range keyCounts {
			scenarios = append(scenarios, subscriptionBenchmarkScenario{
				clients:       clients,
				keys:          keys,
				keysPerClient: keysPerClient,
				parallelism:   parallelism,
				payloadBytes:  payloadBytes,
			})
		}
	}

	return scenarios
}

func openSubscriptionBenchmarkClients(
	b *testing.B,
	serverURL string,
	keys []string,
	keysPerClient int,
	clientCount int,
) ([]*websocket.Conn, []int, *atomic.Uint64, *sync.WaitGroup, chan error) {
	b.Helper()

	wsURL := "ws" + strings.TrimPrefix(serverURL, "http")
	conns := make([]*websocket.Conn, 0, clientCount)
	keySubscribers := make([]int, len(keys))
	received := &atomic.Uint64{}
	readers := &sync.WaitGroup{}
	readErrs := make(chan error, 1)

	for i := 0; i < clientCount; i++ {
		clientKeys := make([]string, 0, keysPerClient)
		for j := 0; j < keysPerClient; j++ {
			keyIdx := (i*keysPerClient + j) % len(keys)
			clientKeys = append(clientKeys, keys[keyIdx])
			keySubscribers[keyIdx]++
		}

		authKey := fmt.Sprintf("bench-auth-%d", i)
		mu.Lock()
		pendingAuthKeys[authKey] = &Pending{
			Keys:      append([]string(nil), clientKeys...),
			ExpiresAt: time.Now().Add(time.Minute),
		}
		mu.Unlock()

		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			closeSubscriptionBenchmarkClients(conns)
			readers.Wait()
			b.Fatalf("dial websocket %d: %v", i, err)
		}

		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			_ = conn.Close()
			closeSubscriptionBenchmarkClients(conns)
			readers.Wait()
			b.Fatalf("set read deadline %d: %v", i, err)
		}
		if err := conn.WriteJSON(map[string]string{"auth_key": authKey}); err != nil {
			_ = conn.Close()
			closeSubscriptionBenchmarkClients(conns)
			readers.Wait()
			b.Fatalf("write auth %d: %v", i, err)
		}

		var subscribed struct {
			Event string   `json:"event"`
			Keys  []string `json:"keys"`
		}
		if err := conn.ReadJSON(&subscribed); err != nil {
			_ = conn.Close()
			closeSubscriptionBenchmarkClients(conns)
			readers.Wait()
			b.Fatalf("read subscribed %d: %v", i, err)
		}
		if err := conn.SetReadDeadline(time.Time{}); err != nil {
			_ = conn.Close()
			closeSubscriptionBenchmarkClients(conns)
			readers.Wait()
			b.Fatalf("clear read deadline %d: %v", i, err)
		}
		if subscribed.Event != "subscribed" || len(subscribed.Keys) != len(clientKeys) {
			_ = conn.Close()
			closeSubscriptionBenchmarkClients(conns)
			readers.Wait()
			b.Fatalf("subscribed response %d = %+v, want %v", i, subscribed, clientKeys)
		}

		readers.Add(1)
		go subscriptionBenchmarkReader(conn, received, readers, readErrs)
		conns = append(conns, conn)
	}

	return conns, keySubscribers, received, readers, readErrs
}

func subscriptionBenchmarkReader(conn *websocket.Conn, received *atomic.Uint64, readers *sync.WaitGroup, readErrs chan error) {
	defer readers.Done()

	for {
		messageType, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) || strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			select {
			case readErrs <- err:
			default:
			}
			return
		}
		if messageType == websocket.TextMessage || messageType == websocket.BinaryMessage {
			received.Add(1)
		}
	}
}

func closeSubscriptionBenchmarkClients(conns []*websocket.Conn) {
	for _, conn := range conns {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "benchmark done"),
			time.Now().Add(time.Second),
		)
		_ = conn.Close()
	}
}

func waitForSubscriptionBenchmarkNotifications(expected int, received *atomic.Uint64, readErrs chan error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if int(received.Load()) >= expected {
			return nil
		}
		select {
		case err := <-readErrs:
			return fmt.Errorf("websocket reader error: %w", err)
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("received %d notifications, want %d", received.Load(), expected)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func subscriptionBenchmarkNotificationCount(totalWrites int, keySubscribers []int) int {
	if totalWrites == 0 || len(keySubscribers) == 0 {
		return 0
	}

	fullCycles := totalWrites / len(keySubscribers)
	remainder := totalWrites % len(keySubscribers)
	total := 0

	for i, subscribers := range keySubscribers {
		writesForKey := fullCycles
		if i < remainder {
			writesForKey++
		}
		total += writesForKey * subscribers
	}

	return total
}

func subscriptionBenchmarkKeys(count int) []string {
	keys := make([]string, count)
	for i := 0; i < count; i++ {
		keys[i] = fmt.Sprintf("bench-key-%03d", i)
	}
	return keys
}

func benchmarkIntEnv(b *testing.B, name string, fallback int) int {
	b.Helper()

	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		b.Fatalf("invalid %s=%q: %v", name, raw, err)
	}
	return value
}

func benchmarkIntListEnv(b *testing.B, name string, fallback []int) []int {
	b.Helper()

	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return append([]int(nil), fallback...)
	}

	parts := strings.Split(raw, ",")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			b.Fatalf("invalid %s=%q: %v", name, raw, err)
		}
		values = append(values, value)
	}

	return values
}
