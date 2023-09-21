package main

import (
	"encoding/json"
	"go-stateful-service/internal/fsm"
	"go-stateful-service/internal/model"
	"log"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

func main() {
	raftPath := os.Getenv("RAFT_PATH")
	if _, err := os.Stat(raftPath); os.IsNotExist(err) {
		if os.Mkdir(raftPath, 0755) != nil {
			log.Fatal("failed to create Raft path")
		}
	}

	raftAddr := os.Getenv("RAFT_ADDR")
	raftID := os.Getenv("RAFT_ID")
	nodeAddrsStr := os.Getenv("RAFT_NODE_ADDRS")
	nodeAddrs := strings.Split(nodeAddrsStr, ",")
	if len(nodeAddrs) == 0 {
		log.Fatal("no Raft node addresses specified")
	}

	serverAddr := os.Getenv("SERVER_ADDR")

	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(raftID)
	config.ElectionTimeout = 2 * time.Second
	config.HeartbeatTimeout = 500 * time.Millisecond

	logStore, err := raftboltdb.NewBoltStore(path.Join(raftPath, "raft-log"))
	if err != nil {
		log.Fatal(err)
	}
	stableStore, err := raftboltdb.NewBoltStore(path.Join(raftPath, "raft-stable"))
	if err != nil {
		log.Fatal(err)
	}

	addr, err := net.ResolveTCPAddr("tcp", raftAddr)
	if err != nil {
		log.Fatal(err)
	}
	transport, err := raft.NewTCPTransport(addr.String(), addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		log.Fatal(err)
	}

	fsmStore := fsm.NewFSM()
	snapshotStore, err := raft.NewFileSnapshotStore(path.Join(raftPath, "raft-snapshots"), 1, os.Stderr)
	if err != nil {
		log.Fatal(err)
	}
	raftNode, err := raft.NewRaft(config, fsmStore, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		log.Fatal(err)
	}

	var servers []raft.Server
	for _, nodeAddr := range nodeAddrs {
		s := strings.Split(nodeAddr, "@")
		if len(s) != 2 {
			log.Fatalf("invalid Raft node address: %s", nodeAddr)
		}
		servers = append(servers, raft.Server{
			ID:      raft.ServerID(s[0]),
			Address: raft.ServerAddress(s[1]),
		})
	}

	f := raftNode.BootstrapCluster(raft.Configuration{
		Servers: servers,
	})
	if err := f.Error(); err != nil {
		log.Println(err)
	}

	app := fiber.New()

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	app.Get("/leader", func(c *fiber.Ctx) error {
		state := raftNode.State()
		if state != raft.Leader {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Not the leader",
			})
		}

		return c.SendStatus(fiber.StatusOK)
	})

	app.Get("/key/:key", func(c *fiber.Ctx) error {
		key := c.Params("key")
		val, ok := fsmStore.Read(key)
		if !ok {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Key not found",
			})
		}
		return c.JSON(model.KeyValue{Key: key, Value: &val})
	})

	app.Post("/key", func(c *fiber.Ctx) error {
		var kv model.KeyValue
		if err := c.BodyParser(&kv); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		b, err := json.Marshal(kv)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to marshal key-value pair",
			})
		}

		if raftNode.State() != raft.Leader {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Not the leader",
			})
		}

		if err := raftNode.Apply(b, 500*time.Millisecond).Error(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to set key-value pair",
			})
		}
		return c.JSON(kv)
	})

	app.Put("/key/:key", func(c *fiber.Ctx) error {
		key := c.Params("key")
		var kv model.KeyValue
		if err := c.BodyParser(&kv); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		b, err := json.Marshal(kv)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to marshal key-value pair",
			})
		}

		if raftNode.State() != raft.Leader {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Not the leader",
			})
		}

		if err := raftNode.Apply(b, 500*time.Millisecond).Error(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update key-value pair",
			})
		}
		return c.JSON(model.KeyValue{Key: key, Value: kv.Value})
	})

	app.Delete("/key/:key", func(c *fiber.Ctx) error {
		key := c.Params("key")
		kv := model.KeyValue{
			Key:   key,
			Value: nil,
		}

		b, err := json.Marshal(kv)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to marshal key-value pair",
			})
		}

		if raftNode.State() != raft.Leader {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Not the leader",
			})
		}

		if err := raftNode.Apply(b, 500*time.Millisecond).Error(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to delete key-value pair",
			})
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	if err := app.Listen(serverAddr); err != nil {
		panic(err)
	}
}
