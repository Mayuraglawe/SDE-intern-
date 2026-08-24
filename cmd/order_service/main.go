package main

import (
	"flag"
	"log"

	"order-position-engine/internal/order"
)

func main() {
	csvFile := flag.String("csv-file", "order_updates (1).csv", "Path to CSV file containing order updates")
	targetURL := flag.String("target-url", "http://localhost:8080/events", "Target URL of Position Service /events endpoint")
	rateLimitMPS := flag.Int("rate-limit-mps", 50, "Rate limit throttling (messages per second)")
	flag.Parse()

	log.Printf("==========================================================")
	log.Printf(" Order Update Service (CSV Producer)")
	log.Printf(" - CSV Source:  %s", *csvFile)
	log.Printf(" - Target URL:  %s", *targetURL)
	log.Printf(" - Throttle:    %d events/sec", *rateLimitMPS)
	log.Printf("==========================================================")

	streamer := order.NewStreamer(order.Config{
		CSVFilePath:  *csvFile,
		TargetURL:    *targetURL,
		RateLimitMPS: *rateLimitMPS,
	})

	stats, err := streamer.Stream()
	if err != nil {
		log.Fatalf("[ORDER SERVICE FATAL] Streaming failed: %v", err)
	}

	log.Printf("[ORDER SERVICE SUCCESS] Stream completed successfully. Processed %d total rows.", stats.TotalRows)
}
