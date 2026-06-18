package parsing

import "testing"

func TestCRC16_StandardVector(t *testing.T) {
	// Canonical CRC-16/CCITT-FALSE check value.
	if got := CRC16([]byte("123456789")); got != 0x29B1 {
		t.Fatalf("CRC16(\"123456789\") = 0x%04X, want 0x29B1", got)
	}
}

func TestCRC16_Empty(t *testing.T) {
	// Empty input returns the init value unchanged.
	if got := CRC16(nil); got != 0xFFFF {
		t.Fatalf("CRC16(empty) = 0x%04X, want 0xFFFF", got)
	}
}

func TestCRC16_SingleByte(t *testing.T) {
	// One byte through the table: init 0xFFFF, process 0x00.
	got := CRC16([]byte{0x00})
	var crc uint16 = 0xFFFF
	want := (crc << 8) ^ crc16Table[byte(crc>>8)]
	if got != want {
		t.Fatalf("CRC16(0x00) = 0x%04X, want 0x%04X", got, want)
	}
}

func TestCRC16_DiffersByInput(t *testing.T) {
	if CRC16([]byte("hello")) == CRC16([]byte("world")) {
		t.Fatal("distinct inputs produced equal CRC")
	}
}
