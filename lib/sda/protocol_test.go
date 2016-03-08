package sda_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dedis/cothority/lib/dbg"
	"github.com/dedis/cothority/lib/network"
	"github.com/dedis/cothority/lib/sda"
	"github.com/dedis/cothority/lib/testutil"
	"github.com/satori/go.uuid"
)

var testID = uuid.NewV5(uuid.NamespaceURL, "test")

// ProtocolTest is the most simple protocol to be implemented, ignoring
// everything it receives.
type ProtocolTest struct {
	*sda.Node
	StartMsg chan string
	DispMsg  chan string
}

// NewProtocolTest is used to create a new protocolTest-instance
func NewProtocolTest(n *sda.Node) (sda.ProtocolInstance, error) {
	return &ProtocolTest{
		Node:     n,
		StartMsg: make(chan string),
		DispMsg:  make(chan string),
	}, nil
}

// Dispatch is used to send the messages further - here everything is
// copied to /dev/null
func (p *ProtocolTest) Dispatch() error {
	dbg.Lvl2("ProtocolTest.Dispatch()")
	p.DispMsg <- "Dispatch"
	return nil
}

func (p *ProtocolTest) Start() error {
	dbg.Lvl2("ProtocolTest.Start()")
	p.StartMsg <- "Start"
	return nil
}

type SimpleProtocol struct {
	// chan to get back to testing
	Chan chan bool
	*sda.Node
}

// Sends a simple message to its first children
func (p *SimpleProtocol) Start() error {
	dbg.Lvl2("Sending from", p.Entity().First(), "to",
		p.Children()[0].Entity.First())
	err := p.SendTo(p.Children()[0], &SimpleMessage{10})
	if err != nil {
		return err
	}
	p.Chan <- true
	return nil
}

// Dispatch analyses the message and does nothing else
func (p *SimpleProtocol) ReceiveMessage(msg struct {
	*sda.TreeNode
	SimpleMessage
}) error {
	dbg.Lvl2("Dispatching", msg)
	if msg.I != 10 {
		return errors.New("Not the value expected")
	}
	p.Chan <- true
	return nil
}

// Test simple protocol-implementation
// - registration
func TestProtocolRegistration(t *testing.T) {
	sda.ProtocolRegister(testID, NewProtocolTest)
	if !sda.ProtocolExists(testID) {
		t.Fatal("Test should exist now")
	}
}

// This makes h2 the leader, so it creates a tree and entity list
// and start a protocol. H1 should receive that message and request the entitity
// list and the treelist and then instantiate the protocol.
func TestProtocolAutomaticInstantiation(t *testing.T) {
	defer testutil.AfterTest(t)

	dbg.TestOutput(testing.Verbose(), 4)
	// setup
	chanH1 := make(chan bool)
	chanH2 := make(chan bool)
	chans := []chan bool{chanH1, chanH2}
	id := 0
	// custom creation function so we know the step due to the channels
	fn := func(n *sda.Node) (sda.ProtocolInstance, error) {
		ps := SimpleProtocol{
			Node: n,
			Chan: chans[id],
		}
		ps.RegisterHandler(ps.ReceiveMessage)
		id++
		return &ps, nil
	}

	network.RegisterMessageType(SimpleMessage{})
	sda.ProtocolRegister(testID, fn)
	h1, h2 := SetupTwoHosts(t, true)
	defer h1.Close()
	defer h2.Close()
	h1.StartProcessMessages()
	// create small Tree
	el := sda.NewEntityList([]*network.Entity{h1.Entity, h2.Entity})
	h1.AddEntityList(el)
	tree := el.GenerateBinaryTree()
	h1.AddTree(tree)
	// start the protocol
	go func() {
		_, err := h1.StartNewNode(testID, tree)
		if err != nil {
			t.Fatal(fmt.Sprintf("Could not start protocol %v", err))
		}
	}()

	// we are supposed to receive something from host1 from Start()
	select {
	case _ = <-chanH1:
		break
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Could not receive from channel of host 1")
	}
	// Then we are supposed to receive from h2 after he got the tree and the
	// entity list from h1
	select {
	case _ = <-chanH2:
		break
	case <-time.After(2 * time.Second):
		t.Fatal("Could not receive from channel of host 1")
	}
}
