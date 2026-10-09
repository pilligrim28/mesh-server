package pulsecom

import "testing"

func TestParseLine(t *testing.T) {
	cases := []struct {
		in   string
		want Msg
	}{
		{"HELLO,MAXPULSE,1.0\r", Msg{Kind: KindHello, Version: "1.0"}},
		{"D,12345,1,72,98,110000,90000", Msg{Kind: KindData, Ms: 12345, Finger: true, BPM: 72, SpO2: 98, IR: 110000, Red: 90000}},
		{"D,5,0,0,0,1200,900", Msg{Kind: KindData, Ms: 5, IR: 1200, Red: 900}},
		{"B,777,65", Msg{Kind: KindBeat, Ms: 777, BPM: 65}},
		{"R,10,100,200", Msg{Kind: KindRaw, Ms: 10, IR: 100, Red: 200}},
		{"S,1,0,1000,31,325", Msg{Kind: KindStatus, Running: true, RateMs: 1000, LED: 31, TempC: 32.5}},
		{"OK,RAW ON", Msg{Kind: KindOK, Text: "RAW ON"}},
		{"ERR,SENSOR_NOT_FOUND", Msg{Kind: KindErr, Text: "SENSOR_NOT_FOUND"}},
	}
	for _, c := range cases {
		got, err := ParseLine(c.in)
		if err != nil || got != c.want {
			t.Errorf("%q: %+v %v, want %+v", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "D,1,2", "B,x,1", "HELLO,OTHER,1", "garbage", "R,1,2,-3"} {
		if _, err := ParseLine(bad); err == nil {
			t.Errorf("%q: ожидалась ошибка", bad)
		}
	}
}

func TestValidCommand(t *testing.T) {
	ok := map[string]string{"ping": "PING", " raw on ": "RAW ON", "rate 500": "RATE 500", "LED 0": "LED 0", "led 255": "LED 255"}
	for in, want := range ok {
		if got, err := ValidCommand(in); err != nil || got != want {
			t.Errorf("%q -> %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"RATE 100", "LED 256", "RESET", "RATE abc", "START\nSTOP"} {
		if _, err := ValidCommand(bad); err == nil {
			t.Errorf("%q: ожидалась ошибка", bad)
		}
	}
}

func TestSanitizeLabel(t *testing.T) {
	cases := map[string]string{"Иван Петров": "Иван_Петров", "../../etc": "etc", "a/b\\c:d*?": "abcd", "": ""}
	for in, want := range cases {
		if got := sanitizeLabel(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}
