package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stenhigh/prifly/internal/local"
	"os"
)

func main() {
	s, err := local.OpenStore(os.Args[1], local.StoreOptions{EventTypes: []string{"test.updated"}})
	if err != nil {
		if errors.Is(err, local.ErrIncompatible) {
			fmt.Println("incompatible")
			return
		}
		panic(err)
	}
	defer s.Close()
	if len(os.Args) > 2 {
		panic("old binary opened migrated storage")
	}
	zero := int64(0)
	_, err = s.Apply(context.Background(), local.Command{ID: "before", Actor: "owner", RunID: "run:stable", Payload: json.RawMessage(`{}`), ExpectedVersion: &zero, Mode: local.CommandCAS}, func(local.Snapshot) (local.Change, error) {
		return local.Change{Data: json.RawMessage(`{"stable":true}`), Events: []local.EventInput{{Type: "test.updated", Data: json.RawMessage(`{}`)}}}, nil
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("ready")
	if !bufio.NewScanner(os.Stdin).Scan() {
		panic("missing signal")
	}
	_, err = s.ApplyAuthority(context.Background(), local.AuthorityCommand{ID: "after", Actor: "owner", Key: "fixture", Payload: json.RawMessage(`{}`)}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) { panic("old writer reached transform") })
	if !errors.Is(err, local.ErrIncompatible) {
		panic(fmt.Sprintf("old writer was not fenced: %v", err))
	}
	fmt.Println("fenced")
}
