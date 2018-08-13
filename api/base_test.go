package api

import (
	"fmt"
	"os"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/Safing/safing-core/container"
	"github.com/Safing/safing-core/formats/varint"
)

func HandleTestCall(call *Call, c *container.Container) {
	msgType, _ := c.GetNextN8()
	switch msgType {
	case API_ACK:
		call.SendAck()
	case API_ERR:
		call.SendError("something went wrong")
	case API_DATA:
		call.SendData(c)
	case API_END:
		call.End()
	}
}

func TestApiBase(t *testing.T) {
	client := APIBase{}
	server := APIBase{}
	up := make(chan *container.Container, 100)
	down := make(chan *container.Container, 100)

	client.Init(false, true, down, up)
	server.Init(true, false, up, down)
	client.RegisterHandler(0, HandleTestCall)
	server.RegisterHandler(0, HandleTestCall)

	go client.Run()
	go server.Run()

	finished := make(chan struct{})
	go func() {
		// wait for test to complete, panic after timeout
		time.Sleep(10 * time.Second)
		select {
		case <-finished:
		default:
			fmt.Println("===== TAKING TOO LONG FOR TEST - PRINTING STACK TRACES =====")
			pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
			os.Exit(1)
		}
	}()

	call := client.Call(0, container.NewContainer(varint.Pack8(API_ACK)))
	response := <-call.Msgs
	handleError(t, response)
	if response.MsgType != API_ACK {
		t.Fatalf("unexpected response msg type %d with data '%s'", response.MsgType, string(response.Container.CompileData()))
	}

	call = client.Call(0, container.NewContainer(varint.Pack8(API_DATA), []byte("test")))
	response = <-call.Msgs
	handleError(t, response)
	if response.MsgType != API_DATA {
		t.Fatalf("unexpected response msg type %d with data '%s'", response.MsgType, string(response.Container.CompileData()))
	}
	testData := string(response.Container.CompileData())
	if testData != "test" {
		t.Fatalf("test data mismatch, got: %s", testData)
	}

	call = client.Call(0, container.NewContainer(varint.Pack8(API_ERR)))
	response = <-call.Msgs
	if response.MsgType != API_ERR {
		t.Fatalf("unexpected response msg type %d with data '%s'", response.MsgType, string(response.Container.CompileData()))
	}

	client.Call(0, container.NewContainer(varint.Pack8(API_END)))

	client.Shutdown()
	server.Shutdown()

	close(finished)
}

func handleError(t *testing.T, msg *ApiMsg) {
	if msg.MsgType == API_ERR {
		t.Fatalf("received error: %s", ParseError(msg.Container))
	}
}
