// Copyright 2019 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

const version = "0.3.0"

func usage(exeType int) {
	switch exeType {
	case subExe:
		log.Printf("Usage: nats-sub [-s server] [-creds file] [-t] <subject>\n")
	case reqExe:
		log.Printf("Usage: nats-req [-s server] [-creds file] [-t] <subject> <request>\n")
	case repExe:
		log.Printf("Usage: nats-rply [-s server] [-creds file] [-t] [-q queue] <subject> <response>\n")
	default:
		log.Printf("Usage: nats-pub [-s server] [-creds file] [-t] <subject> <msg>\n")
	}
	flag.PrintDefaults()
}

func main() {
	var urls = flag.String("s", stringFromEnv("NATS_URL", "connect.ngs.global"), "The NATS System")
	var userCreds = flag.String("creds", stringFromEnv("NATS_CREDS", ""), "User Credentials File")
	var queue = flag.String("q", "NATS-RPLY-22", "Queue Group Name")
	var showTime = flag.Bool("t", false, "Display timestamps")
	var showHelp = flag.Bool("h", false, "Show help message")
	var showVersion = flag.Bool("v", false, "Show version")

	exeType := exeType()

	log.SetFlags(0)
	flag.Usage = func() { usage(exeType) }
	flag.Parse()

	if *showHelp {
		usage(exeType)
		os.Exit(0)
	}

	if *showVersion {
		log.Printf("nats-box v%s", version)
		os.Exit(0)
	}

	args := flag.Args()

	if exeType != subExe && len(args) != 2 || exeType == subExe && len(args) != 1 {
		usage(exeType)
		os.Exit(1)
	}

	// Connect Options.
	opts := []nats.Option{nats.Name(toolName(exeType))}
	opts = setupConnOptions(opts)

	// Use UserCredentials
	if *userCreds != "" {
		opts = append(opts, nats.UserCredentials(*userCreds))
	}

	us := *urls
	if v := os.Getenv("NATS_URL"); v != "" {
		us = v
	}

	// Connect to NATS
	nc, err := nats.Connect(us, opts...)
	if err != nil {
		log.Fatal(err)
	}

	switch exeType {
	case subExe:
		subj, i := args[0], 0

		nc.Subscribe(subj, func(msg *nats.Msg) {
			i++
			printMsg(msg, i)
		})
		nc.Flush()
		if err := nc.LastError(); err != nil {
			log.Fatal(err)
		}
		log.Printf("Listening on [%s]", subj)
		if *showTime {
			log.SetFlags(log.LstdFlags)
		}
	case reqExe:
		subj, reqMsg := args[0], []byte(args[1])
		msg, err := nc.Request(subj, reqMsg, 2*time.Second)
		if err != nil {
			if nc.LastError() != nil {
				log.Fatalf("%v for request", nc.LastError())
			}
			log.Fatalf("%v for request", err)
		}
		fmt.Printf("%s\n", msg.Data)
	case repExe:
		subj, repMsg, i := args[0], []byte(args[1]), 0
		nc.QueueSubscribe(subj, *queue, func(msg *nats.Msg) {
			i++
			printMsg(msg, i)
			msg.Respond(repMsg)
		})
		nc.Flush()
		if err := nc.LastError(); err != nil {
			log.Fatal(err)
		}
		log.Printf("Listening on [%s %s]", subj, *queue)
		if *showTime {
			log.SetFlags(log.LstdFlags)
		}
	default:
		subj, msg := args[0], []byte(args[1])
		nc.Publish(subj, msg)
		nc.Flush()
		if err := nc.LastError(); err != nil {
			log.Fatal(err)
		}
	}

	if exeType == subExe || exeType == repExe {
		runtime.Goexit()
	}
}

func printMsg(m *nats.Msg, i int) {
	log.Printf("[#%d] Received on [%s]: '%s'", i, m.Subject, m.Data)
}

// Mostly for nats-sub only.
func setupConnOptions(opts []nats.Option) []nats.Option {
	totalWait := 10 * time.Minute
	reconnectDelay := time.Second

	opts = append(opts, nats.ReconnectWait(reconnectDelay))
	opts = append(opts, nats.MaxReconnects(int(totalWait/reconnectDelay)))
	opts = append(opts, nats.DisconnectHandler(func(nc *nats.Conn) {
		log.Printf("Disconnected: will attempt reconnects for %.0fm", totalWait.Minutes())
	}))
	opts = append(opts, nats.ReconnectHandler(func(nc *nats.Conn) {
		log.Printf("Reconnected [%s]", nc.ConnectedUrl())
	}))
	opts = append(opts, nats.ClosedHandler(func(nc *nats.Conn) {
		log.Fatalf("Exiting: %v", nc.LastError())
	}))
	return opts
}

const (
	pubExe = iota
	subExe
	reqExe
	repExe
)

func exeType() int {
	exeName := strings.ToLower(filepath.Base(filepath.Clean(os.Args[0])))
	if len(exeName) < 7 {
		return pubExe
	}
	switch exeName[len(exeName)-4:] {
	case "-pub":
		return pubExe
	case "-sub":
		return subExe
	case "-req":
		return reqExe
	case "rply":
		return repExe
	}
	return pubExe
}

func toolName(exeType int) string {
	switch exeType {
	case subExe:
		return "NATS-SUB TOOL"
	case reqExe:
		return "NATS-REQ TOOL"
	case repExe:
		return "NATS-RPLY TOOL"
	default:
		return "NATS-PUB TOOL"
	}
}

func stringFromEnv(v string, d string) string {
	val := os.Getenv(v)
	if val == "" {
		return d
	}

	return val
}
