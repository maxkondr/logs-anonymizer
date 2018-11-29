package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	sipanonymizer "github.com/maxkondr/porta-sip-anonymizer"
)

// ErrResponse renderer type for handling all sorts of errors.
//
// In the best case scenario, the excellent github.com/pkg/errors package
// helps reveal information on the error, setting it on Err, and in the Render()
// method, using it to set the application-specific error code in AppCode.
type ErrResponse struct {
	Err            error `json:"-"` // low-level runtime error
	HTTPStatusCode int   `json:"-"` // http response status code

	StatusText string `json:"status"`          // user-level status message
	AppCode    int64  `json:"code,omitempty"`  // application-specific error code
	ErrorText  string `json:"error,omitempty"` // application-level error message, for debugging
}

func (e *ErrResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, e.HTTPStatusCode)
	return nil
}

func ErrInvalidRequest(err error) render.Renderer {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: 400,
		StatusText:     "Invalid request.",
		ErrorText:      err.Error(),
	}
}

type LogEntryParticipant struct {
	Alias         string `json:"alias"`
	Host          string `json:"host"`
	Name          string `json:"name"`
	ParticipantID string `json:"participant_id"`
	ProductName   string `json:"product_name"`
	SipNode       string `json:"sip_node"`
}

type GlobalLogMetaInfo struct {
	SessionList            []string              `json:"session_list"`
	H323ConfID             string                `json:"h323_conf_id"`
	ParticipantList        []LogEntryParticipant `json:"participant_list"`
	NodeList               []string              `json:"node_list"`
	CallID                 string                `json:"call_id"`
	CLDList                []string              `json:"cld_list"`
	CLIList                []string              `json:"cli_list"`
	IAccount               []string              `json:"i_account"`
	ICustomer              []string              `json:"i_customer"`
	ParentBillingSessionID string                `json:"parent_billing_session_id"`
}

type LogEntryMetaInfo struct {
	DateTime          string `json:"datetime"`
	DiagramText       string `json:"diagramtext"`
	DialogID          string `json:"dialog_id"`
	Level             int    `json:"level"`
	MessageClass      string `json:"message_class"`
	Operation         string `json:"operation"`
	ParticipantFrom   string `json:"participant_from"`
	ParticipantFromID string `json:"participant_from_id"`
	ParticipantTo     string `json:"participant_to"`
	ParticipantToID   string `json:"participant_to_id"`
	SipNode           string `json:"sip_node"`
}

type SipLogEntry struct {
	MetaInfo      LogEntryMetaInfo `json:"meta_info"`
	SipLogMessage string           `json:"text"`
	Type          string           `json:"type"`
}

type SipLogEntryRequest struct {
	LogEntries     []SipLogEntry     `json:"log_message_list"`
	GlobalMetaInfo GlobalLogMetaInfo `json:"meta_info"`
}

func (m *SipLogEntryRequest) Bind(r *http.Request) error {
	// if len(m.LogEntries) == 0 {
	// 	return errors.New("missing required LogEntries data")
	// }
	return nil
}

type SipLogEntryResponse struct {
	LogEntries     []SipLogEntry     `json:"log_message_list"`
	GlobalMetaInfo GlobalLogMetaInfo `json:"meta_info"`
}

func processSipLogEntry1(entry *SipLogEntry) SipLogEntry {
	var parsedText []byte
	// parsedText := entry.SipLogMessage
	if entry.Type == "sip" {
		// parsedText = .sipanonymizer.ProcessMessage([]byte(entry.SipLogMessage))
		parsedText = []byte(entry.SipLogMessage)
		sipanonymizer.ProcessMessage(parsedText)
	}
	// else {
	// 	parsedText = entry.SipLogMessage
	// }

	return SipLogEntry{MetaInfo: entry.MetaInfo,
		Type:          entry.Type,
		SipLogMessage: string(parsedText)}
}

func NewSipLogEntryResponse1(req *SipLogEntryRequest) *SipLogEntryResponse {
	resp := &SipLogEntryResponse{GlobalMetaInfo: req.GlobalMetaInfo}
	resp.LogEntries = make([]SipLogEntry, 0, len(req.LogEntries))

	for _, entry := range req.LogEntries {
		// respEntry := processSipLogEntry1(&entry)
		// resp.LogEntries = append(resp.LogEntries, respEntry)
		resp.LogEntries = append(resp.LogEntries, processSipLogEntry1(&entry))
	}
	return resp
}

func processSipLogEntry2(entry *SipLogEntry) *SipLogEntry {
	// var parsedText string
	if entry.Type == "sip" {
		// parsedText = sipanonymizer.ProcessMessage([]byte(entry.SipLogMessage))
		sipanonymizer.ProcessMessage([]byte(entry.SipLogMessage))
	}
	// } else {
	// 	parsedText = entry.SipLogMessage
	// }

	return &SipLogEntry{MetaInfo: entry.MetaInfo,
		Type:          entry.Type,
		SipLogMessage: entry.SipLogMessage}
}

type channelResult struct {
	ID       int
	LogEntry *SipLogEntry
}

func NewSipLogEntryResponse2(req *SipLogEntryRequest) *SipLogEntryResponse {
	resp := &SipLogEntryResponse{GlobalMetaInfo: req.GlobalMetaInfo}
	eLen := len(req.LogEntries)
	resp.LogEntries = make([]SipLogEntry, 0, eLen)
	c := make(chan channelResult, eLen)

	var wg sync.WaitGroup

	for i, entry := range req.LogEntries {
		wg.Add(1)

		go func(c chan<- channelResult, i int, entry *SipLogEntry) {
			defer wg.Done()
			result := channelResult{ID: i, LogEntry: processSipLogEntry2(entry)}
			select {
			case c <- result:
			}
		}(c, i, &entry)
	}
	wg.Wait()
	close(c)

	m := make(map[int]*SipLogEntry, eLen)

	for res := range c {
		m[res.ID] = res.LogEntry
	}

	for i := 0; i < len(req.LogEntries); i++ {
		resp.LogEntries = append(resp.LogEntries, *m[i])
	}
	return resp
}

func (rd *SipLogEntryResponse) Render(w http.ResponseWriter, r *http.Request) error {
	// Pre-processing before a response is marshalled and sent across the wire
	return nil
}

func sipHandler(w http.ResponseWriter, r *http.Request) {
	sipMsgReq := &SipLogEntryRequest{}
	if err := render.Bind(r, sipMsgReq); err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}

	resp := NewSipLogEntryResponse1(sipMsgReq)
	// resp := NewSipLogEntryResponse2(sipMsgReq)

	if resp != nil {
		render.Status(r, http.StatusOK)
		render.Render(w, r, resp)
	} else {
		render.Status(r, http.StatusBadRequest)
		render.Render(w, r, nil)
	}
}

func main() {
	flag.Parse()

	r := chi.NewRouter()

	// r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// r.Use(middleware.URLFormat)
	r.Use(render.SetContentType(render.ContentTypeJSON))

	r.Post("/sip", sipHandler)

	fmt.Println("Start listening on port 3333")
	go func() {
		if err := http.ListenAndServe(":3333", r); err != nil {
			fmt.Println("Error: ", err)
		}
	}()

	// Listen for OS signals
	ch := make(chan os.Signal, 10)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	close(ch)

	fmt.Println("Finished")
}
