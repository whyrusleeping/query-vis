package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	notif "github.com/ipfs/go-ipfs/notifications"
	//kb "github.com/ipfs/go-ipfs/routing/kbucket"
	//peer "github.com/ipfs/go-ipfs/p2p/peer"

	. "github.com/whyrusleeping/stump"
)

type QueryOp struct {
	active  map[string]time.Time
	toQuery map[string]struct{}
	dialing map[string]time.Time

	begin time.Time
	took  time.Duration

	goodDials []time.Duration
	failDials []time.Duration
	responses []time.Duration
}

func NewQueryOp() *QueryOp {
	return &QueryOp{
		active:  make(map[string]time.Time),
		toQuery: make(map[string]struct{}),
		dialing: make(map[string]time.Time),
		begin:   time.Now(),
	}
}

func (qo *QueryOp) HandleQueryEvent(qe notif.QueryEvent) {
	switch qe.Type {
	case notif.SendingQuery:
		t, ok := qo.dialing[qe.ID.Pretty()]
		if ok {
			took := time.Now().Sub(t)
			qo.goodDials = append(qo.goodDials, took)
			Log("dial %s took %s", qe.ID, took)
			delete(qo.dialing, qe.ID.Pretty())
		}
		Log("querying: ", qe.ID)
		qo.active[qe.ID.Pretty()] = time.Now()
		delete(qo.toQuery, qe.ID.Pretty())
	case notif.PeerResponse:
		t, ok := qo.active[qe.ID.Pretty()]
		if !ok {
			Fatal("received response from peer we didnt query")
		}

		took := time.Now().Sub(t)
		qo.responses = append(qo.responses, took)

		Log("response from: %s in %s", qe.ID, took)
		Log("peerinfos")
		delete(qo.active, qe.ID.Pretty())
		for _, pe := range qe.Responses {
			Log("\t%s [%d]", pe.ID.Pretty(), len(pe.Addrs))
		}
	case notif.Value:
		Log("value from: ", qe.ID)
		qo.took = time.Now().Sub(qo.begin)
	case notif.AddingPeer:
		qo.toQuery[qe.ID.Pretty()] = struct{}{}
	case notif.DialingPeer:
		qo.dialing[qe.ID.Pretty()] = time.Now()
	case notif.QueryError:
		Error("query error: %s %s", qe.ID, qe.Extra)
		t, ok := qo.dialing[qe.ID.Pretty()]
		if ok {
			took := time.Now().Sub(t)
			qo.failDials = append(qo.failDials, took)
			Error("dial failed in %s", took)
			delete(qo.dialing, qe.ID.Pretty())
		}
	default:
		Log("unrecognized type: ", qe.Type)
	}
}
func (qo *QueryOp) PrintFinal() {
	Log("\n\n\nquery finished in", qo.took)
	Log("dialed a total of %d peers", len(qo.goodDials)+len(qo.failDials))
	Log("%d dials failed in an average of %s", len(qo.failDials), averageTimes(qo.failDials))
	Log("%d dials succeeded in an average of %s", len(qo.goodDials), averageTimes(qo.goodDials))
	Log("received a total of %d responses, average time %s", len(qo.responses), averageTimes(qo.responses))
	Log("no responses from %d peers", len(qo.active))
	for k, _ := range qo.active {
		Log("  - %s", k)
	}
	Log("%d peers unqueried", len(qo.toQuery))
	Log("%d dials pending at finish", len(qo.dialing))
}

func main() {
	host := "http://localhost:5001"

	if len(os.Args) < 2 {
		Fatal("must specify key to query")
	}
	key := os.Args[1]

	qo := NewQueryOp()
	url := fmt.Sprintf("%s/api/v0/dht/get?v=true&arg=%s", host, key)
	resp, err := http.Get(url)
	if err != nil {
		Fatal(err)
	}

	dec := json.NewDecoder(resp.Body)
	for {
		var qe notif.QueryEvent
		err := dec.Decode(&qe)
		if err != nil {
			if err == io.EOF {
				Log("complete")
				return
			}
			Fatal(err)
		}

		qo.HandleQueryEvent(qe)
	}

	qo.PrintFinal()
}

func averageTimes(ts []time.Duration) time.Duration {
	if len(ts) == 0 {
		return 0
	}
	var sum time.Duration
	for _, t := range ts {
		sum += t
	}
	return sum / time.Duration(len(ts))
}
