package parsing

// CRC-16/CCITT-FALSE: poly=0x1021, init=0xFFFF, refIn=false, refOut=false,
// xorOut=0x0000. Test vector: CRC16("123456789") == 0x29B1.
//
// Ported 1:1 from the C# Crc16Ccitt (table-driven, MSB-first).

var crc16Table = buildCRC16Table()

func buildCRC16Table() [256]uint16 {
	var table [256]uint16
	for i := 0; i < 256; i++ {
		crc := uint16(i) << 8
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}

// CRC16 computes the CRC-16/CCITT-FALSE checksum over data.
func CRC16(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc = (crc << 8) ^ crc16Table[byte(crc>>8)^b]
	}
	return crc
}
