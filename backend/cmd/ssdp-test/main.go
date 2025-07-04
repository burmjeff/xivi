package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	SSDPMulticastAddr = "239.255.255.250:1900"
	DefaultTimeout    = 5 * time.Second
)

func main() {
	// Configure logging
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Command line flags
	var (
		command = flag.String("cmd", "discover", "Command to run: discover, listen, send")
		timeout = flag.Duration("timeout", DefaultTimeout, "Timeout for operations")
		st      = flag.String("st", "ssdp:all", "Search target for M-SEARCH requests")
		verbose = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	if *verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	switch *command {
	case "discover":
		runDiscovery(*st, *timeout)
	case "listen":
		runListener(*timeout)
	case "send":
		sendMSearch(*st)
	case "interfaces":
		listNetworkInterfaces()
	default:
		fmt.Printf("Unknown command: %s\n", *command)
		fmt.Println("Available commands: discover, listen, send, interfaces")
		os.Exit(1)
	}
}

// runDiscovery sends M-SEARCH requests and listens for responses
func runDiscovery(searchTarget string, timeout time.Duration) {
	log.Info().Msgf("Starting SSDP discovery for ST: %s", searchTarget)

	// Create UDP connection for listening to responses
	listenAddr, err := net.ResolveUDPAddr("udp4", ":0")
	if err != nil {
		log.Fatal().Msgf("Failed to resolve listen address: %v", err)
	}

	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		log.Fatal().Msgf("Failed to create UDP listener: %v", err)
	}
	defer conn.Close()

	// Send M-SEARCH request
	if err := sendMSearchRequest(searchTarget); err != nil {
		log.Fatal().Msgf("Failed to send M-SEARCH request: %v", err)
	}

	log.Info().Msgf("M-SEARCH request sent, listening for responses for %v...", timeout)

	// Listen for responses
	responses := 0
	conn.SetReadDeadline(time.Now().Add(timeout))
	buffer := make([]byte, 1024)

	for {
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			log.Error().Msgf("Error reading UDP packet: %v", err)
			continue
		}

		response := string(buffer[:n])
		if strings.HasPrefix(response, "HTTP/1.1 200 OK") {
			responses++
			log.Info().Msgf("Response #%d from %s:", responses, addr.String())
			printHTTPMessage(response)
		}
	}

	if responses == 0 {
		log.Warn().Msg("No SSDP responses received")
	} else {
		log.Info().Msgf("Received %d SSDP responses", responses)
	}
}

// runListener listens for SSDP traffic on the multicast address
func runListener(timeout time.Duration) {
	log.Info().Msgf("Listening for SSDP traffic on %s for %v...", SSDPMulticastAddr, timeout)

	// Resolve multicast address
	addr, err := net.ResolveUDPAddr("udp4", SSDPMulticastAddr)
	if err != nil {
		log.Fatal().Msgf("Failed to resolve multicast address: %v", err)
	}

	// Create UDP connection
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		log.Fatal().Msgf("Failed to listen on multicast address: %v", err)
	}
	defer conn.Close()

	// Join multicast group on all interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Fatal().Msgf("Failed to get network interfaces: %v", err)
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagMulticast != 0 {
			if err := conn.JoinGroup(&iface, addr); err != nil {
				log.Warn().Msgf("Failed to join multicast group on %s: %v", iface.Name, err)
			} else {
				log.Debug().Msgf("Joined multicast group on interface: %s", iface.Name)
			}
		}
	}

	// Listen for packets
	messages := 0
	conn.SetReadDeadline(time.Now().Add(timeout))
	buffer := make([]byte, 1024)

	for {
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			log.Error().Msgf("Error reading UDP packet: %v", err)
			continue
		}

		message := string(buffer[:n])
		messages++

		if strings.HasPrefix(message, "M-SEARCH") {
			log.Info().Msgf("M-SEARCH #%d from %s:", messages, addr.String())
		} else if strings.HasPrefix(message, "NOTIFY") {
			log.Info().Msgf("NOTIFY #%d from %s:", messages, addr.String())
		} else {
			log.Info().Msgf("Message #%d from %s:", messages, addr.String())
		}

		printHTTPMessage(message)
	}

	if messages == 0 {
		log.Warn().Msg("No SSDP traffic received")
	} else {
		log.Info().Msgf("Received %d SSDP messages", messages)
	}
}

// sendMSearch sends a single M-SEARCH request
func sendMSearch(searchTarget string) {
	log.Info().Msgf("Sending M-SEARCH request for ST: %s", searchTarget)

	if err := sendMSearchRequest(searchTarget); err != nil {
		log.Fatal().Msgf("Failed to send M-SEARCH request: %v", err)
	}

	log.Info().Msg("M-SEARCH request sent successfully")
}

// sendMSearchRequest sends an M-SEARCH request to the multicast address
func sendMSearchRequest(searchTarget string) error {
	// Create UDP connection
	conn, err := net.Dial("udp", SSDPMulticastAddr)
	if err != nil {
		return fmt.Errorf("failed to create UDP connection: %v", err)
	}
	defer conn.Close()

	// Build M-SEARCH request
	request := fmt.Sprintf("M-SEARCH * HTTP/1.1\r\n"+
		"HOST: %s\r\n"+
		"MAN: \"ssdp:discover\"\r\n"+
		"ST: %s\r\n"+
		"MX: 3\r\n"+
		"\r\n",
		SSDPMulticastAddr,
		searchTarget)

	// Send request
	_, err = conn.Write([]byte(request))
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}

	log.Debug().Msgf("Sent M-SEARCH request:\n%s", request)
	return nil
}

// listNetworkInterfaces displays information about network interfaces
func listNetworkInterfaces() {
	log.Info().Msg("Network interfaces:")

	interfaces, err := net.Interfaces()
	if err != nil {
		log.Fatal().Msgf("Failed to get network interfaces: %v", err)
	}

	for _, iface := range interfaces {
		flags := []string{}
		if iface.Flags&net.FlagUp != 0 {
			flags = append(flags, "UP")
		}
		if iface.Flags&net.FlagLoopback != 0 {
			flags = append(flags, "LOOPBACK")
		}
		if iface.Flags&net.FlagMulticast != 0 {
			flags = append(flags, "MULTICAST")
		}

		fmt.Printf("Interface: %s\n", iface.Name)
		fmt.Printf("  Flags: %s\n", strings.Join(flags, ", "))
		fmt.Printf("  Hardware Address: %s\n", iface.HardwareAddr.String())

		// Get addresses
		addrs, err := iface.Addrs()
		if err != nil {
			fmt.Printf("  Error getting addresses: %v\n", err)
		} else {
			for _, addr := range addrs {
				fmt.Printf("  Address: %s\n", addr.String())
			}
		}
		fmt.Println()
	}
}

// printHTTPMessage formats and prints an HTTP-style message
func printHTTPMessage(message string) {
	lines := strings.Split(message, "\r\n")
	for i, line := range lines {
		if i == 0 {
			// First line (request/response line)
			fmt.Printf("  %s\n", line)
		} else if line == "" {
			// Empty line indicates end of headers
			break
		} else {
			// Header line
			fmt.Printf("  %s\n", line)
		}
	}
	fmt.Println()
}

// printJSON prints a struct as formatted JSON
func printJSON(v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Error().Msgf("Failed to marshal JSON: %v", err)
		return
	}
	fmt.Println(string(data))
}
