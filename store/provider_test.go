package store

import (
	"os"
	"testing"
	"time"

	command "github.com/rqlite/rqlite/v8/command/proto"
)

// Test_SingleNodeProvide tests that the Store correctly implements
// the Provide method.
func Test_SingleNodeProvide(t *testing.T) {
	s0, ln := mustNewStore(t)
	defer ln.Close()

	if err := s0.Open(); err != nil {
		t.Fatalf("failed to open single-node store: %s", err.Error())
	}
	if err := s0.Bootstrap(NewServer(s0.ID(), s0.Addr(), true)); err != nil {
		t.Fatalf("failed to bootstrap single-node store: %s", err.Error())
	}
	defer s0.Close(true)
	if _, err := s0.WaitForLeader(10 * time.Second); err != nil {
		t.Fatalf("Error waiting for leader: %s", err)
	}

	er := executeRequestFromStrings([]string{
		`CREATE TABLE foo (id INTEGER NOT NULL PRIMARY KEY, name TEXT)`,
		`INSERT INTO foo(id, name) VALUES(1, "fiona")`,
	}, false, false)
	_, err := s0.Execute(er)
	if err != nil {
		t.Fatalf("failed to execute on single node: %s", err.Error())
	}
	qr := queryRequestFromString("SELECT * FROM foo", false, false)
	qr.Level = command.QueryRequest_QUERY_REQUEST_LEVEL_NONE
	r, err := s0.Query(qr)
	if err != nil {
		t.Fatalf("failed to query leader node: %s", err.Error())
	}
	if exp, got := `["id","name"]`, asJSON(r[0].Columns); exp != got {
		t.Fatalf("unexpected results for query\nexp: %s\ngot: %s", exp, got)
	}
	if exp, got := `[[1,"fiona"]]`, asJSON(r[0].Values); exp != got {
		t.Fatalf("unexpected results for query\nexp: %s\ngot: %s", exp, got)
	}

	tempFile := mustCreateTempFile()
	defer os.Remove(tempFile)
	provider := NewProvider(s0, false)
	if _, err := provider.Provide(tempFile); err != nil {
		t.Fatalf("failed to provide SQLite data: %s", err.Error())
	}

	// Load the provided data into a new store and check it.
	s1, ln := mustNewStore(t)
	defer ln.Close()

	if err := s1.Open(); err != nil {
		t.Fatalf("failed to open single-node store: %s", err.Error())
	}
	if err := s1.Bootstrap(NewServer(s1.ID(), s1.Addr(), true)); err != nil {
		t.Fatalf("failed to bootstrap single-node store: %s", err.Error())
	}
	defer s1.Close(true)
	if _, err := s1.WaitForLeader(10 * time.Second); err != nil {
		t.Fatalf("Error waiting for leader: %s", err)
	}

	err = s1.Load(loadRequestFromFile(tempFile))
	if err != nil {
		t.Fatalf("failed to load provided SQLite data: %s", err.Error())
	}
	qr = queryRequestFromString("SELECT * FROM foo", false, false)
	qr.Level = command.QueryRequest_QUERY_REQUEST_LEVEL_STRONG
	r, err = s1.Query(qr)
	if err != nil {
		t.Fatalf("failed to query leader node: %s", err.Error())
	}
	if exp, got := `["id","name"]`, asJSON(r[0].Columns); exp != got {
		t.Fatalf("unexpected results for query\nexp: %s\ngot: %s", exp, got)
	}
	if exp, got := `[[1,"fiona"]]`, asJSON(r[0].Values); exp != got {
		t.Fatalf("unexpected results for query\nexp: %s\ngot: %s", exp, got)
	}
}

// Test_SingleNodeProvideNoData checks the Provide method operates
// correctly when there is no data to provide.
func Test_SingleNodeProvideNoData(t *testing.T) {
	s, ln := mustNewStore(t)
	defer ln.Close()

	if err := s.Open(); err != nil {
		t.Fatalf("failed to open single-node store: %s", err.Error())
	}
	if err := s.Bootstrap(NewServer(s.ID(), s.Addr(), true)); err != nil {
		t.Fatalf("failed to bootstrap single-node store: %s", err.Error())
	}
	defer s.Close(true)
	if _, err := s.WaitForLeader(10 * time.Second); err != nil {
		t.Fatalf("Error waiting for leader: %s", err)
	}

	tmpFile := mustCreateTempFile()
	defer os.Remove(tmpFile)
	provider := NewProvider(s, false)
	if _, err := provider.Provide(tmpFile); err != nil {
		t.Fatalf("store failed to provide: %s", err.Error())
	}
}

func Test_SingleNodeProvideLastModified(t *testing.T) {
	s, ln := mustNewStore(t)
	defer ln.Close()

	if err := s.Open(); err != nil {
		t.Fatalf("failed to open single-node store: %s", err.Error())
	}
	if err := s.Bootstrap(NewServer(s.ID(), s.Addr(), true)); err != nil {
		t.Fatalf("failed to bootstrap single-node store: %s", err.Error())
	}
	defer s.Close(true)
	if _, err := s.WaitForLeader(10 * time.Second); err != nil {
		t.Fatalf("Error waiting for leader: %s", err)
	}

	tmpFile := mustCreateTempFile()
	defer os.Remove(tmpFile)
	provider := NewProvider(s, false)

	lm, err := provider.LastModified()
	if err != nil {
		t.Fatalf("failed to get last modified: %s", err.Error())
	}

	er := executeRequestFromStrings([]string{
		`CREATE TABLE foo (id INTEGER NOT NULL PRIMARY KEY, name TEXT)`,
		`INSERT INTO foo(id, name) VALUES(1, "fiona")`,
	}, false, false)
	_, err = s.Execute(er)
	if err != nil {
		t.Fatalf("failed to execute on single node: %s", err.Error())
	}
	if _, err := s.WaitForAppliedFSM(2 * time.Second); err != nil {
		t.Fatalf("failed to wait for FSM to apply")
	}

	newLM, err := provider.LastModified()
	if err != nil {
		t.Fatalf("failed to get last modified: %s", err.Error())
	}
	if !newLM.After(lm) {
		t.Fatalf("last modified time should have changed")
	}
	lm = newLM

	// Try various queries and commands which should not change the database.
	qr := queryRequestFromString("SELECT * FROM foo", false, false)
	qr.Level = command.QueryRequest_QUERY_REQUEST_LEVEL_STRONG
	_, err = s.Query(qr)
	if err != nil {
		t.Fatalf("failed to query leader node: %s", err.Error())
	}
	if _, err := s.WaitForAppliedFSM(2 * time.Second); err != nil {
		t.Fatalf("failed to wait for FSM to apply")
	}
	newLM, err = provider.LastModified()
	if err != nil {
		t.Fatalf("failed to get last modified: %s", err.Error())
	}
	if !newLM.Equal(lm) {
		t.Fatalf("last modified time should not have changed")
	}
	lm = newLM

	if af, err := s.Noop("don't care"); err != nil || af.Error() != nil {
		t.Fatalf("failed to execute Noop")
	}
	newLM, err = provider.LastModified()
	if err != nil {
		t.Fatalf("failed to get last modified: %s", err.Error())
	}
	if !newLM.Equal(lm) {
		t.Fatalf("last modified time should not have changed")
	}
	lm = newLM

	er = executeRequestFromStrings([]string{
		`INSERT INTO foo(id, name) VALUES(1, "fiona")`, // Constraint violation.
	}, false, false)
	_, err = s.Execute(er)
	if err != nil {
		t.Fatalf("failed to execute on single node: %s", err.Error())
	}
	newLM, err = provider.LastModified()
	if err != nil {
		t.Fatalf("failed to get last modified: %s", err.Error())
	}
	if !newLM.Equal(lm) {
		t.Fatalf("last modified time should not have changed with constraint violation")
	}
	lm = newLM

	// This should change the database.
	er = executeRequestFromStrings([]string{
		`INSERT INTO foo(id, name) VALUES(2, "fiona")`,
	}, false, false)
	_, err = s.Execute(er)
	if err != nil {
		t.Fatalf("failed to execute on single node: %s", err.Error())
	}
	if _, err := s.WaitForAppliedFSM(2 * time.Second); err != nil {
		t.Fatalf("failed to wait for FSM to apply")
	}
	newLM, err = provider.LastModified()
	if err != nil {
		t.Fatalf("failed to get last modified: %s", err.Error())
	}
	if !newLM.After(lm) {
		t.Fatalf("last modified time should have changed")
	}
}
