package main

import (
	"testing"
	"time"

	"github.com/spezifisch/stmps/logger"
)

func TestNewCache(t *testing.T) {
	logger := logger.Logger{}

	t.Run("basic string cache creation", func(t *testing.T) {
		zero := "empty"
		c := NewCache(
			zero,
			func(k string) (string, error) { return zero, nil },
			func(k, v string) {},
			func(k string) bool { return false },
			time.Second,
			&logger,
		)
		defer c.Close()
		if c.zero != zero {
			t.Errorf("expected %q, got %q", zero, c.zero)
		}
		if c.cache == nil || len(c.cache) != 0 {
			t.Errorf("expected non-nil, empty map; got %#v", c.cache)
		}
		if c.pipeline == nil {
			t.Errorf("expected non-nil chan; got %#v", c.pipeline)
		}
	})

	t.Run("different data type cache creation", func(t *testing.T) {
		zero := -1
		c := NewCache(
			zero,
			func(k string) (int, error) { return zero, nil },
			func(k string, v int) {},
			func(k string) bool { return false },
			time.Second,
			&logger,
		)
		defer c.Close()
		if c.zero != zero {
			t.Errorf("expected %d, got %d", zero, c.zero)
		}
		if c.cache == nil || len(c.cache) != 0 {
			t.Errorf("expected non-nil, empty map; got %#v", c.cache)
		}
		if c.pipeline == nil {
			t.Errorf("expected non-nil chan; got %#v", c.pipeline)
		}
	})
}

func TestGet(t *testing.T) {
	logger := logger.Logger{}
	zero := "zero"
	items := map[string]string{"a": "1", "b": "2", "c": "3"}
	c := NewCache(
		zero,
		func(k string) (string, error) {
			return items[k], nil
		},
		func(k, v string) {},
		func(k string) bool { return false },
		time.Second,
		&logger,
	)
	defer c.Close()
	t.Run("empty cache get returns zero", func(t *testing.T) {
		got := c.Get("a")
		if got != zero {
			t.Errorf("expected %q, got %q", zero, got)
		}
	})
	// Give the fetcher a chance to populate the cache
	time.Sleep(time.Millisecond)
	t.Run("non-empty cache get returns value", func(t *testing.T) {
		got := c.Get("a")
		expected := "1"
		if got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})
}

func TestCallback(t *testing.T) {
	logger := logger.Logger{}
	zero := "zero"
	var gotK, gotV string
	expectedK := "a"
	expectedV := "1"
	c := NewCache(
		zero,
		func(k string) (string, error) {
			return expectedV, nil
		},
		func(k, v string) {
			gotK = k
			gotV = v
		},
		func(k string) bool { return false },
		time.Second,
		&logger,
	)
	defer c.Close()
	t.Run("callback gets called back", func(t *testing.T) {
		c.Get(expectedK)
		// Give the callback goroutine a chance to do its thing
		time.Sleep(time.Millisecond)
		if gotK != expectedK {
			t.Errorf("expected key %q, got %q", expectedV, gotV)
		}
		if gotV != expectedV {
			t.Errorf("expected value %q, got %q", expectedV, gotV)
		}
	})
}

func TestClose(t *testing.T) {
	logger := logger.Logger{}
	t.Run("pipeline is closed", func(t *testing.T) {
		c0 := NewCache(
			"",
			func(k string) (string, error) { return "A", nil },
			func(k, v string) {},
			func(k string) bool { return false },
			time.Second,
			&logger,
		)
		// Put something in the cache
		c0.Get("")
		// Give the cache time to populate the cache
		time.Sleep(time.Millisecond)
		// Make sure the cache isn't empty
		if len(c0.cache) == 0 {
			t.Fatalf("expected the cache to be non-empty, but it was. Probably a threading issue with the test, and we need a longer timeout.")
		}
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on pipeline use; got none")
			}
		}()
		c0.Close()
		if len(c0.cache) > 0 {
			t.Errorf("expected empty cache; was %d", len(c0.cache))
		}
		c0.Get("")
	})

	t.Run("callback gets called back", func(t *testing.T) {
		c0 := NewCache(
			"",
			func(k string) (string, error) { return "", nil },
			func(k, v string) {},
			func(k string) bool { return false },
			time.Second,
			&logger,
		)
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on pipeline use; got none")
			}
		}()
		c0.Close()
		c0.Get("")
	})
}

func TestInvalidate(t *testing.T) {
	logger := logger.Logger{}
	zero := "zero"
	var gotV string
	expected := "1"
	c := NewCache(
		zero,
		func(k string) (string, error) {
			return expected, nil
		},
		func(k, v string) {
			gotV = v
		},
		func(k string) bool {
			return true
		},
		500*time.Millisecond,
		&logger,
	)
	defer c.Close()
	t.Run("basic invalidation", func(t *testing.T) {
		if c.Get("a") != zero {
			t.Errorf("expected %q, got %q", zero, gotV)
		}
		// Give the callback goroutine a chance to do its thing
		time.Sleep(time.Millisecond)
		if c.Get("a") != expected {
			t.Errorf("expected %q, got %q", expected, gotV)
		}
		// Give the invalidation time to be called
		time.Sleep(600 * time.Millisecond)
		if c.Get("a") != zero {
			t.Errorf("expected %q, got %q", zero, gotV)
		}
	})
}
