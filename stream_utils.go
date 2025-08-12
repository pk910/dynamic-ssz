package dynssz

import (
	"encoding/binary"
	"io"
)

// ---- Stream Marshal functions ----

// writeBool writes a boolean to the writer
func writeBool(w io.Writer, b bool) error {
	var buf [1]byte
	if b {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	_, err := w.Write(buf[:])
	return err
}

// writeUint8 writes a uint8 to the writer
func writeUint8(w io.Writer, i uint8) error {
	var buf [1]byte
	buf[0] = byte(i)
	_, err := w.Write(buf[:])
	return err
}

// writeUint16 writes a little endian uint16 to the writer
func writeUint16(w io.Writer, i uint16) error {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], i)
	_, err := w.Write(buf[:])
	return err
}

// writeUint32 writes a little endian uint32 to the writer
func writeUint32(w io.Writer, i uint32) error {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], i)
	_, err := w.Write(buf[:])
	return err
}

// writeUint64 writes a little endian uint64 to the writer
func writeUint64(w io.Writer, i uint64) error {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], i)
	_, err := w.Write(buf[:])
	return err
}

// writeZeroPadding writes the specified number of zero bytes to the writer
func writeZeroPadding(w io.Writer, count int) error {
	if len(zeroBytes) == 0 {
		zeroBytes = make([]byte, 1024)
	}
	for count > 0 {
		toCopy := count
		if toCopy > len(zeroBytes) {
			toCopy = len(zeroBytes)
		}
		_, err := w.Write(zeroBytes[:toCopy])
		if err != nil {
			return err
		}
		count -= toCopy
	}
	return nil
}

// ---- Stream Unmarshal functions ----

// readBool reads a boolean from the reader
func readBool(r io.Reader) (bool, error) {
	var buf [1]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return false, err
	}
	return buf[0] != 0, nil
}

// readUint8 reads a uint8 from the reader
func readUint8(r io.Reader) (uint8, error) {
	var buf [1]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return buf[0], nil
}

// readUint16 reads a little endian uint16 from the reader
func readUint16(r io.Reader) (uint16, error) {
	var buf [2]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(buf[:]), nil
}

// readUint32 reads a little endian uint32 from the reader
func readUint32(r io.Reader) (uint32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}

// readUint64 reads a little endian uint64 from the reader
func readUint64(r io.Reader) (uint64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}