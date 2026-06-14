package swpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestRetry503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestGetKIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"time_tag":"2026-06-14T12:00:00","Kp":1.0,"a_running":3,"station_count":6},
			{"time_tag":"2026-06-14T13:00:00","Kp":3.5,"a_running":5,"station_count":7},
			{"time_tag":"2026-06-14T14:00:00","Kp":5.0,"a_running":7,"station_count":8},
			{"time_tag":"2026-06-14T15:00:00","Kp":8.2,"a_running":9,"station_count":9}
		]`))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	records, err := c.GetKIndex(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 {
		t.Fatalf("got %d records, want 4", len(records))
	}

	cases := []struct {
		kp       float64
		activity string
	}{
		{1.0, "Quiet"},
		{3.5, "Unsettled"},
		{5.0, "Minor Storm"},
		{8.2, "Strong Storm"},
	}
	for i, tc := range cases {
		if records[i].Kp != tc.kp {
			t.Errorf("records[%d].Kp = %f, want %f", i, records[i].Kp, tc.kp)
		}
		if records[i].Activity != tc.activity {
			t.Errorf("records[%d].Activity = %q, want %q", i, records[i].Activity, tc.activity)
		}
	}
}

func TestGetKIndexLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"time_tag":"2026-06-14T12:00:00","Kp":1.0,"a_running":3,"station_count":6},
			{"time_tag":"2026-06-14T13:00:00","Kp":2.0,"a_running":4,"station_count":7},
			{"time_tag":"2026-06-14T14:00:00","Kp":3.0,"a_running":5,"station_count":8}
		]`))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	records, err := c.GetKIndex(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records with limit=2, want 2", len(records))
	}
	// limit takes the last N records
	if records[0].Time != "2026-06-14T13:00:00" {
		t.Errorf("first record time = %q, want 2026-06-14T13:00:00", records[0].Time)
	}
}

func TestGetSolarWind(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"proton_speed":469,"time_tag":"2026-06-14T18:42:00"}]`))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	sw, err := c.GetSolarWind(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sw.ProtonSpeed != 469 {
		t.Errorf("ProtonSpeed = %d, want 469", sw.ProtonSpeed)
	}
	if sw.Time != "2026-06-14T18:42:00" {
		t.Errorf("Time = %q, want 2026-06-14T18:42:00", sw.Time)
	}
}

func TestGetAlerts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"product_id":"ALTEF3","issue_datetime":"2026 Jun 14 1328 UTC","message":"Space Weather Message Code: ALTEF3\nSerial Number: 3700"},
			{"product_id":"WATA20","issue_datetime":"2026 Jun 14 1400 UTC","message":"Space Weather Message Code: WATA20\nSerial Number: 1234"}
		]`))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	alerts, err := c.GetAlerts(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 2 {
		t.Fatalf("got %d alerts, want 2", len(alerts))
	}
	if alerts[0].ProductID != "ALTEF3" {
		t.Errorf("ProductID = %q, want ALTEF3", alerts[0].ProductID)
	}
	if alerts[0].Message == "" {
		t.Error("Message is empty")
	}
}

func TestGetFlares(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"time_tag":"2026-06-13T10:00:00","begin_time":"2026-06-13T10:00:00","begin_class":"C1.5","max_time":"2026-06-13T10:05:00","max_class":"C2.1","end_time":"2026-06-13T10:10:00","end_class":"C1.0","satellite":16},
			{"time_tag":"2026-06-14T08:00:00","begin_time":"2026-06-14T08:00:00","begin_class":"M1.2","max_time":"2026-06-14T08:08:00","max_class":"M2.0","end_time":"2026-06-14T08:15:00","end_class":"M1.0","satellite":16},
			{"time_tag":"2026-06-12T15:00:00","begin_time":"2026-06-12T15:00:00","begin_class":"B5.0","max_time":"2026-06-12T15:03:00","max_class":"B7.0","end_time":"2026-06-12T15:06:00","end_class":"B4.0","satellite":18}
		]`))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	flares, err := c.GetFlares(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(flares) != 3 {
		t.Fatalf("got %d flares, want 3", len(flares))
	}
	// Should be sorted descending by begin_time
	if flares[0].BeginTime != "2026-06-14T08:00:00" {
		t.Errorf("flares[0].BeginTime = %q, want most recent", flares[0].BeginTime)
	}
	if flares[0].BeginClass != "M1.2" {
		t.Errorf("flares[0].BeginClass = %q, want M1.2", flares[0].BeginClass)
	}
	if flares[2].BeginTime != "2026-06-12T15:00:00" {
		t.Errorf("flares[2].BeginTime = %q, want oldest", flares[2].BeginTime)
	}
}

func TestGetFlaresLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"begin_time":"2026-06-14T08:00:00","begin_class":"M1.2","max_time":"2026-06-14T08:08:00","max_class":"M2.0","end_time":"2026-06-14T08:15:00","satellite":16},
			{"begin_time":"2026-06-13T10:00:00","begin_class":"C1.5","max_time":"2026-06-13T10:05:00","max_class":"C2.1","end_time":"2026-06-13T10:10:00","satellite":16},
			{"begin_time":"2026-06-12T15:00:00","begin_class":"B5.0","max_time":"2026-06-12T15:03:00","max_class":"B7.0","end_time":"2026-06-12T15:06:00","satellite":18}
		]`))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	flares, err := c.GetFlares(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(flares) != 2 {
		t.Fatalf("got %d flares with limit=2, want 2", len(flares))
	}
}

func TestKpActivity(t *testing.T) {
	cases := []struct {
		kp       float64
		activity string
	}{
		{0.5, "Quiet"},
		{1.0, "Quiet"},
		{2.0, "Unsettled"},
		{3.5, "Unsettled"},
		{4.0, "Active"},
		{4.9, "Active"},
		{5.0, "Minor Storm"},
		{6.5, "Minor Storm"},
		{7.0, "Moderate Storm"},
		{7.9, "Moderate Storm"},
		{8.0, "Strong Storm"},
		{8.9, "Strong Storm"},
		{9.0, "Extreme Storm"},
	}
	for _, tc := range cases {
		got := kpActivity(tc.kp)
		if got != tc.activity {
			t.Errorf("kpActivity(%v) = %q, want %q", tc.kp, got, tc.activity)
		}
	}
}
