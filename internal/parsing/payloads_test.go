package parsing

import "testing"

func TestDecodeGnss_Valid(t *testing.T) {
	p, err := decodeGnss(gnssBody(12.34, -5.67, 2.10, 13.57, 1247.5, 0x04))
	if err != nil {
		t.Fatalf("unexpected error: %v", err.Message)
	}
	g := p.(GnssPayload)
	if g.FixQuality != "rtk_fixed" {
		t.Fatalf("fix_quality = %s, want rtk_fixed", g.FixQuality)
	}
	if g.DisplacementXMm != 12.34 {
		t.Fatalf("disp_x = %v, want 12.34", g.DisplacementXMm)
	}
}

func TestDecodeGnss_FixQualityMap(t *testing.T) {
	want := []string{"no_fix", "single", "dgps", "rtk_float", "rtk_fixed"}
	for b, exp := range want {
		p, err := decodeGnss(gnssBody(0, 0, 0, 0, 0, byte(b)))
		if err != nil {
			t.Fatalf("fix byte %d: %v", b, err.Message)
		}
		if got := p.(GnssPayload).FixQuality; got != exp {
			t.Fatalf("fix byte %d -> %s, want %s", b, got, exp)
		}
	}
}

func TestDecodeGnss_Errors(t *testing.T) {
	if _, err := decodeGnss(make([]byte, 20)); err == nil {
		t.Fatal("short payload accepted")
	}
	if _, err := decodeGnss(gnssBody(0, 0, 0, -1, 0, 0)); err == nil {
		t.Fatal("negative horizontal accepted")
	}
	if _, err := decodeGnss(gnssBody(0, 0, 0, 0, -1, 0)); err == nil {
		t.Fatal("negative cumulative accepted")
	}
	if _, err := decodeGnss(gnssBody(0, 0, 0, 0, 0, 0x05)); err == nil {
		t.Fatal("unknown fix byte accepted")
	}
}

func TestDecodeRainfall(t *testing.T) {
	body := append(append(f32be(2.4), f32be(1547.2)...), 0x00, 0x3C) // interval 60
	p, err := decodeRainfall(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err.Message)
	}
	r := p.(RainfallPayload)
	if r.IntervalMinutes != 60 {
		t.Fatalf("interval = %d, want 60", r.IntervalMinutes)
	}
	// interval out of range
	bad := append(append(f32be(0), f32be(0)...), 0x05, 0xDC) // 1500 > 1440
	if _, err := decodeRainfall(bad); err == nil {
		t.Fatal("interval 1500 accepted")
	}
}

func TestDecodeStrain_WithAndWithoutTemp(t *testing.T) {
	noTemp := append([]byte{0x00}, f32be(234.5)...)
	p, err := decodeStrain(noTemp)
	if err != nil {
		t.Fatalf("no-temp error: %v", err.Message)
	}
	if p.(StrainPayload).TemperatureCelsius != nil {
		t.Fatal("temperature should be nil when HAS_TEMP=0x00")
	}

	withTemp := append(append([]byte{0x01}, f32be(234.5)...), f32be(24.3)...)
	p, err = decodeStrain(withTemp)
	if err != nil {
		t.Fatalf("with-temp error: %v", err.Message)
	}
	tc := p.(StrainPayload).TemperatureCelsius
	if tc == nil || *tc != 24.3 {
		t.Fatalf("temperature = %v, want 24.3", tc)
	}

	// invalid HAS_TEMP
	if _, err := decodeStrain(append([]byte{0x02}, f32be(0)...)); err == nil {
		t.Fatal("invalid HAS_TEMP accepted")
	}
	// temp out of range
	bad := append(append([]byte{0x01}, f32be(0)...), f32be(100)...)
	if _, err := decodeStrain(bad); err == nil {
		t.Fatal("temp 100 accepted")
	}
}

func TestDecodeSoilPressure(t *testing.T) {
	withTemp := append(append([]byte{0x01}, f32be(45.2)...), f32be(18.7)...)
	p, err := decodeSoilPressure(withTemp)
	if err != nil {
		t.Fatalf("error: %v", err.Message)
	}
	sp := p.(SoilPressurePayload)
	if sp.PressureKpa != 45.2 || sp.TemperatureCelsius == nil || *sp.TemperatureCelsius != 18.7 {
		t.Fatalf("got %+v", sp)
	}
}

func TestDecodeDeepDisplacement(t *testing.T) {
	body := make([]byte, 0, 16)
	body = append(body, f32be(15)...)
	body = append(body, f32be(3.21)...)
	body = append(body, f32be(-1.54)...)
	body = append(body, f32be(48.7)...)
	p, err := decodeDeepDisplacement(body)
	if err != nil {
		t.Fatalf("error: %v", err.Message)
	}
	d := p.(DeepDisplacementPayload)
	if d.DepthM != 15 {
		t.Fatalf("depth = %v, want 15", d.DepthM)
	}
	// negative depth
	neg := make([]byte, 16)
	copy(neg, f32be(-1))
	if _, err := decodeDeepDisplacement(neg); err == nil {
		t.Fatal("negative depth accepted")
	}
}
