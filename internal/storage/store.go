package storage

import (
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	defaultBucket = "livesync-bridge"
)

// Store provides persistent key-value storage using BoltDB
type Store struct {
	db *bolt.DB
}

// NewStore creates a new persistent storage instance
func NewStore(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create default bucket
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(defaultBucket))
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Set stores a key-value pair
func (s *Store) Set(key, value string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(defaultBucket))
		return b.Put([]byte(key), []byte(value))
	})
}

// Get retrieves a value by key
func (s *Store) Get(key string) (string, error) {
	var value string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(defaultBucket))
		v := b.Get([]byte(key))
		if v == nil {
			return fmt.Errorf("key not found: %s", key)
		}
		value = string(v)
		return nil
	})
	return value, err
}

// Has checks if a key exists
func (s *Store) Has(key string) bool {
	var exists bool
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(defaultBucket))
		v := b.Get([]byte(key))
		exists = (v != nil)
		return nil
	})
	return exists
}

// Delete removes a key-value pair
func (s *Store) Delete(key string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(defaultBucket))
		return b.Delete([]byte(key))
	})
}

// Clear removes all key-value pairs
func (s *Store) Clear() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket([]byte(defaultBucket)); err != nil {
			return err
		}
		_, err := tx.CreateBucket([]byte(defaultBucket))
		return err
	})
}

// Keys returns all keys in the store
func (s *Store) Keys() ([]string, error) {
	var keys []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(defaultBucket))
		return b.ForEach(func(k, v []byte) error {
			keys = append(keys, string(k))
			return nil
		})
	})
	return keys, err
}

// GetWithDefault retrieves a value or returns a default if not found
func (s *Store) GetWithDefault(key, defaultValue string) string {
	value, err := s.Get(key)
	if err != nil {
		return defaultValue
	}
	return value
}
