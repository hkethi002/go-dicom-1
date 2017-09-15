// Package main is a script that reads a filesystem full of dcm files and
// generates a json report.
package dcmdump

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"errors"

	"github.com/davidgamba/go-dicom/dcmdump/tag"
	"github.com/davidgamba/go-dicom/dcmdump/ts"
	vri "github.com/davidgamba/go-dicom/dcmdump/vr"
)

var debug bool
var ErrNotDICM = errors.New("Not a Dicom File")

func debugf(format string, a ...interface{}) (n int, err error) {
	if debug {
		return fmt.Printf(format, a...)
	}
	return 0, nil
}
func debugln(a ...interface{}) (n int, err error) {
	if debug {
		return // fmt.Println(a...)
	}
	return 0, nil
}

type stringSlice []string

func (s stringSlice) contains(a string) bool {
	for _, b := range s {
		if a == b {
			return true
		}
	}
	return false
}

type dicomqr struct {
	Empty [128]byte
	DICM  [4]byte
	Rest  []byte
}

// DataElement -
type DataElement struct {
	N        int
	TagGroup []byte // [2]byte
	TagElem  []byte // [2]byte
	TagStr   string
	Name 	 string
	VR       []byte // [2]byte
	VRStr    string
	VRLen    int
	Len      uint32
	Data     []byte
	PartOfSQ bool
}

// DicomFile -
type DicomFile struct {
	Elements []DataElement
	Path string
}

// Look up element by tag string or Name
func (file *DicomFile) LookupElement(name string) (*DataElement, error) {

	for _, elem := range file.Elements {
		if elem.TagStr == name {
			return &elem, nil
		}
	}
	for _, elem := range file.Elements {
		if elem.Name == name {
			return &elem, nil
		}
	}

	return nil, errors.New("Could not find tag in dicom dictionary")
}

// String -
func (de *DataElement) String() string {
	tn := tag.Tag[de.TagStr]["name"]
	if _, ok := tag.Tag[de.TagStr]; !ok {
		tn = "MISSING"
	}
	padding := ""
	if de.PartOfSQ {
		padding = "    "
	}
	if de.Len < 128 {
		return fmt.Sprintf("%s%04d (%s) %s %d %d %s %s", padding, de.N, de.TagStr, de.VRStr, de.VRLen, de.Len, tn, de.stringData())
	}
	return fmt.Sprintf("%s%04d (%s) %s %d %d %s %s", padding, de.N, de.TagStr, de.VRStr, de.VRLen, de.Len, tn, "...")
}

type fh os.File

// func readNBytes(f *os.File, size int) ([]byte, error) {
// 	data := make([]byte, size)
// 	for {
// 		data = data[:cap(data)]
// 		n, err := f.Read(data)
// 		if err != nil {
// 			if err == io.EOF {
// 				break
// 			}
// 			return nil, err
// 		}
// 		data = data[:n]
// 	}
// 	return data, nil
// }

// http://rosettacode.org/wiki/Strip_control_codes_and_extended_characters_from_a_string#Go
// two UTF-8 functions identical except for operator comparing c to 127
func stripCtlFromUTF8(str string) string {
	return strings.Map(func(r rune) rune {
		if r >= 32 && r != 127 {
			return r
		}
		return '.'
	}, str)
}

func tagString(b []byte) string {
	tag := strings.ToUpper(fmt.Sprintf("%02x%02x%02x%02x", b[1], b[0], b[3], b[2]))
	return tag
}

func printBytes(b []byte) {
	if !debug {
		return
	}
	l := len(b)
	var s string
	for i := 0; i < l; i++ {
		s += stripCtlFromUTF8(string(b[i]))
		if i != 0 && i%8 == 0 {
			if i%16 == 0 {
				fmt.Printf(" - %s\n", s)
				s = ""
			} else {
				fmt.Printf(" - ")
			}
		}
		fmt.Printf("%2x ", b[i])
		if i == l-1 {
			if 15-i%16 > 7 {
				fmt.Printf(" - ")
			}
			for j := 0; j < 15-i%16; j++ {
				// fmt.Printf("   ")
				fmt.Printf("   ")
			}
			fmt.Printf(" - %s\n", s)
			s = ""
		}
	}
	fmt.Printf("\n")
}
func (de *DataElement) StringData() string {
	return de.stringData()
}

func (de *DataElement) stringData() string {
	if de.TagStr == "00020010" {
		dataStr := string(de.Data)
		l := len(de.Data)
		if de.Data[l-1] == 0x0 {
			dataStr = string(de.Data[:l-1])
		}
		if tsStr, ok := ts.TS[dataStr]; ok {
			return dataStr + " " + tsStr["name"].(string)
		}
	}
	if _, ok := vri.VR[de.VRStr]["fixed"]; ok && vri.VR[de.VRStr]["fixed"].(bool) {
		s := ""
		l := len(de.Data)
		n := 0
		vrl := vri.VR[de.VRStr]["len"].(int)
		switch vrl {
		case 1:
			for n+1 <= l {
				s += fmt.Sprintf("%d ", de.Data[n])
				n++
			}
			return s
		case 2:
			for n+2 <= l {
				e := binary.LittleEndian.Uint16(de.Data[n : n+2])
				s += fmt.Sprintf("%d ", e)
				n += 2
			}
			return s
		case 4:
			for n+4 <= l {
				e := binary.LittleEndian.Uint32(de.Data[n : n+4])
				s += fmt.Sprintf("%d ", e)
				n += 4
			}
			return s
		default:
			return string(de.Data)
		}
	} else {
		if _, ok := vri.VR[de.VRStr]["padded"]; ok && vri.VR[de.VRStr]["padded"].(bool) {
			l := len(de.Data)
			if de.Data[l-1] == 0x0 {
				return string(de.Data[:l-1])
			}
			return string(de.Data)
		}
		return string(de.Data)
	}
}

func readNbytes (f *os.File, size int, off int) ([]byte, error) {
	buff := make([]byte, size)
	n, err := f.ReadAt(buff, int64(off))
	if err != nil {
		return buff, err
	} else if n != size {
		return buff, errors.New("Number of read byte does not equal given size")
	}
	return buff, nil
}

func parseDataElement(path string, n int, explicit bool, limit int, tags []string) ([]DataElement, error) {
	l := limit
	// Data element
	m := n
	elements := make([]DataElement,0)
	dfile, err := os.Open(path)
	if err != nil {
		return elements, err
	}

	for n <= l && m+4 <= l && n <= limit && m+4 <= limit {
		undefinedLen := false
		de := DataElement{N: n}
		m += 4
		t, err := readNbytes(dfile, 4, n)
		if err != nil {
			return elements, err
		}
		de.TagGroup = t[:2]
		de.TagElem = t[2:]
		de.TagStr = tagString(t)
		// TODO: Clean up tagString
		tagStr := tagString(t)
		n = m
		if tagStr == "" {
		} else if _, ok := tag.Tag[tagStr]; !ok {
			// fmt.Fprintf(os.Stderr, "INFO: %d Missing tag '%s'\n", n, tagStr)
		} else {
			de.Name = tag.Tag[tagStr]["name"]
		}
		var len uint32
		var vr string
		if explicit {
			m += 2
			vr_byte, err := readNbytes(dfile, 2, n)
			if err != nil {
				return elements, err
			}
			de.VR = vr_byte
			de.VRStr = string(vr_byte)
			vr = string(vr_byte)
			if _, ok := vri.VR[vr]; !ok {
				if vr_byte[0] == 0x0 && vr_byte[1] == 0x0 {
					// fmt.Fprintf(os.Stderr, "INFO: Blank VR\n")
					vr = "00"
					de.VRStr = "00"
				} else {
					// fmt.Fprintf(os.Stderr, "ERROR: %d Missing VR '%s'\n", n, vr)
					return elements, err
				}
			}
			n = m
			if vr == "OB" ||
				vr == "OD" ||
				vr == "OF" ||
				vr == "OL" ||
				vr == "OW" ||
				vr == "SQ" ||
				vr == "UC" ||
				vr == "UR" ||
				vr == "UT" ||
				vr == "UN" {
				m += 2
				n = m
				m += 4
				bytes, err := readNbytes(dfile, m-n, n)
				if err != nil {
					return elements, err
				}
				len = binary.LittleEndian.Uint32(bytes)
				n = m
			} else {
				m += 2
				bytes, err := readNbytes(dfile, m-n, n)
				if err != nil {
					return elements, err
				}
				len16 := binary.LittleEndian.Uint16(bytes)
				len = uint32(len16)
				n = m
			}
		} else {
			m += 4
			bytes, err := readNbytes(dfile, m-n, n)
			if err != nil {
				return elements, err
			}
			len = binary.LittleEndian.Uint32(bytes)
			n = m
		}
		if len == 0xFFFFFFFF {
			undefinedLen = true
			for {
				endTag, err := readNbytes(dfile, 4, m)
				if err != nil {
					return elements, err
				}
				endTagStr := tagString(endTag)
				if de.TagStr == "FFFEE000" && endTagStr == "FFFEE00D" {
					// FFFEE000 item
					// find FFFEE00D: ItemDelimitationItem
					len = uint32(m - n)
					m = n
					break
				} else if endTagStr == "FFFEE0DD" {
					// Find FFFEE0DD: SequenceDelimitationItem
					len = uint32(m - n)
					m = n
					break
				} else {
					m++
					if m >= l {
						// fmt.Fprintf(os.Stderr, "ERROR: Couldn't find SequenceDelimitationItem\n")
						return elements, err
					}
				}
			}
		}
		de.Len = len
		debugf("Lenght: %d\n", len)
		m += int(len)
		if de.TagStr == "7FE00010" {
			de.Data = []byte{}
		} else if de.TagStr == "FFFEE000" {
			de.Data = []byte{}
			// fmt.Println(de.String())
			parseDataElement(path, n, true, m, tags)
		} else if vr == "SQ" {
			de.Data = []byte{}
			// fmt.Println(de.String())
			parseDataElement(path, n, false, m, tags)
		} else if stringInSlice(de.TagStr, tags) {
			if m < limit && m < l {
				de.Data, err = readNbytes(dfile, m-n, n)
				if err != nil {
					return elements, err
				}
			}
			if de.TagStr == "0020000E" {
				m = l
			}
			// fmt.Println(de.String())
		}
		if undefinedLen {
			m += 8
		}
		n = m
		// if de.Name != "PixelData"{
		// 	elements = append(elements, de)
		// }
		if stringInSlice(de.TagStr, tags) {
			elements = append(elements, de)
		}
	}
	dfile.Close()
	return elements, err
}

func stringInSlice(a string, tags []string) bool {
    for _, b := range tags {
        if b == a {
            return true
        }
    }
    return false
}

func (di *DicomFile) ProcessFile(path string, m int, explicit bool, tags []string) error {
	fi, err := os.Stat(path);
	if err != nil {
	    return err
	}
	// get the size
	size := fi.Size()
	di.Elements, err = parseDataElement(path, m, explicit, int(size), tags)
	return err
}
