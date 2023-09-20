package main

import (
	"context"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func main() {
	app := fiber.New()

	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	_, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		panic(err)
	}

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	app.Get("/key/:key", func(c *fiber.Ctx) error {
		key := c.Params("key")
		val, err := rdb.Get(c.Context(), key).Result()
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Key not found",
			})
		}
		return c.JSON(KeyValue{Key: key, Value: val})
	})

	app.Post("/key", func(c *fiber.Ctx) error {
		var kv KeyValue
		if err := c.BodyParser(&kv); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}
		err = rdb.Set(c.Context(), kv.Key, kv.Value, 0).Err()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to set key-value pair",
			})
		}
		return c.JSON(kv)
	})

	app.Put("/key/:key", func(c *fiber.Ctx) error {
		key := c.Params("key")
		var kv KeyValue
		if err := c.BodyParser(&kv); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}
		err = rdb.Set(c.Context(), key, kv.Value, 0).Err()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update key-value pair",
			})
		}
		return c.JSON(KeyValue{Key: key, Value: kv.Value})
	})

	app.Delete("/key/:key", func(c *fiber.Ctx) error {
		key := c.Params("key")
		err := rdb.Del(c.Context(), key).Err()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to delete key-value pair",
			})
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	if err := app.Listen(":4000"); err != nil {
		panic(err)
	}
}
