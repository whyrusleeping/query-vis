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

func main() {
	host := "http://localhost:5001"

	if len(os.Args) < 2 {
		Fatal("must specify key to query")
	}
	key := os.Args[1]

	begin := time.Now()
	url := fmt.Sprintf("%s/api/v0/dht/get?v=true&arg=%s", host, key)
	resp, err := http.Get(url)
	if err != nil {
		Fatal(err)
	}

	dec := json.NewDecoder(resp.Body)
	active := make(map[string]time.Time)
	toQuery := make(map[string]struct{})
	dialing := make(map[string]time.Time)
	var goodDials []time.Duration
	var failDials []time.Duration
	var responses []time.Duration
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

		switch qe.Type {
		case notif.SendingQuery:
			t, ok := dialing[qe.ID.Pretty()]
			if ok {
				took := time.Now().Sub(t)
				goodDials = append(goodDials, took)
				Log("dial %s took %s", qe.ID, took)
				delete(dialing, qe.ID.Pretty())
			}
			Log("querying: ", qe.ID)
			active[qe.ID.Pretty()] = time.Now()
			delete(toQuery, qe.ID.Pretty())
		case notif.PeerResponse:
			t, ok := active[qe.ID.Pretty()]
			if !ok {
				Fatal("received response from peer we didnt query")
			}

			took := time.Now().Sub(t)
			responses = append(responses, took)

			Log("response from: %s in %s", qe.ID, took)
			Log("peerinfos")
			delete(active, qe.ID.Pretty())
			for _, pe := range qe.Responses {
				Log("\t%s [%d]", pe.ID.Pretty(), len(pe.Addrs))
			}
		case notif.Value:
			took := time.Now().Sub(begin)
			Log("\n\n\nquery finished in", took)
			Log("dialed a total of %d peers", len(goodDials)+len(failDials))
			Log("%d dials failed in an average of %s", len(failDials), averageTimes(failDials))
			Log("%d dials succeeded in an average of %s", len(goodDials), averageTimes(goodDials))
			Log("received a total of %d responses, average time %s", len(responses), averageTimes(responses))
			Log("no responses from %d peers", len(active))
			Log("%d peers unqueried", len(toQuery))
			Log("%d dials pending at finish", len(dialing))
		case notif.AddingPeer:
			toQuery[qe.ID.Pretty()] = struct{}{}
		case notif.DialingPeer:
			dialing[qe.ID.Pretty()] = time.Now()
		case notif.QueryError:
			Error("query error: %s %s", qe.ID, qe.Extra)
			t, ok := dialing[qe.ID.Pretty()]
			if ok {
				took := time.Now().Sub(t)
				failDials = append(failDials, took)
				Error("dial failed in %s", took)
				delete(dialing, qe.ID.Pretty())
			}
		default:
			Log("unrecognized type: ", qe.Type)
		}
	}
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
