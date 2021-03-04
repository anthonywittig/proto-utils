package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var (
	lineRegx = regexp.MustCompile(`(\S+)\s+(\S+)\s+(\S+)\s+(\S+)`)
)

func main() {
	err := generateStuf()
	if err != nil {
		panic(err)
	}
}

func generateStuf() error {
	var wamProto strings.Builder
	var wamStruct strings.Builder
	var wamCurl strings.Builder
	var noop strings.Builder

	wamProto.WriteString(`
message CreateWAMDataRequest {
  salesforceenums.Consumer consumer = 1;
  WAMData wam_data= 2;
}

message CreateWAMDataResponse {
  WAMData wam_data = 1;
}`)

	wamProto.WriteString("\n\n")
	wamProto.WriteString("message WAMData {\n")

	wamStruct.WriteString("type WAMData struct {\n")

	wamCurl.WriteString("{\n")

	r := NewLineReader(rawData())
	// burn the header
	_, _ = r.next()

	wamDataProtoNum := 1
	for {
		line, ok := r.nextSkipSpace()
		if !ok {
			break
		}

		m := lineRegx.FindStringSubmatch(line)
		if len(m) != 5 {
			return fmt.Errorf("non-matching line: \"%s\", count: %d", line, len(m))
		}
		sfObject := strings.Replace(m[1], "__c", "", 1)
		rawSFName := m[2]
		_ = m[3] // JS name but we won't use this version of it.
		rawType := m[4]

		pName := strings.Replace(rawSFName, "__c", "", 1)
		pName = strings.ToLower(pName)

		jName := pName

		fName := strings.ToUpper(string(jName[0])) + jName[1:]
		fName = strings.Replace(fName, "Id", "ID", 1)

		curlVal := ""
		pType := ""
		sType := ""
		if rawType == "string" {
			pType = "string"
			sType = "string"
			if rawSFName == "Account__c" {
				curlVal = "\"0011N0000194gsW\""
			} else if rawSFName == "Parent_Account__c" {
				curlVal = "\"\""
			} else if rawSFName == "Primary_Opportunity__c" {
				curlVal = "\"0063l00000iDy81\""
			} else if strings.Contains(rawSFName, "_Email__c") {
				curlVal = "\"test@getweave.com\""
			} else if strings.Contains(rawSFName, "_Time_") {
				curlVal = "\"12:00:00\""
			} else if strings.Contains(rawSFName, "_Date_") || strings.Contains(rawSFName, "_At_") {
				curlVal = "\"2021-02-11T21:27:25Z\""
			} else if _, ok := map[string]struct{}{
				"Install_Technician__c":       {},
				"Network_Router_Admin__c":     {},
				"Business_Contact__c":         {},
				"Business_Primary_Contact__c": {},
				"Business_Owner__c":           {},
				"Form_Progress_User_Id__c":    {},
			}[rawSFName]; ok {
				curlVal = "\"\""
			} else if strings.Contains(rawSFName, "Weave_User__c") || strings.Contains(rawSFName, "User_Id__c") {
				curlVal = "\"" + uuid.New().String() + "\""
				//curlVal = "\"\""
			} else {
				curlVal = fmt.Sprintf("\"%s test\"", rawSFName)
				if len(curlVal) > 10 {
					curlVal = curlVal[0:9] + "\""
				}
			}
		} else if rawType == "boolean" {
			pType = "bool"
			sType = "bool"
			curlVal = fmt.Sprintf("true")
		} else if rawType == "number" {
			pType = "float"
			sType = "float32"
			curlVal = fmt.Sprintf("%d", wamDataProtoNum)
		}
		if pType == "" {
			return fmt.Errorf("unhandled type inference for: %s", rawType)
		}

		protoWriter := &wamProto
		protoNumber := -1
		switch sfObject {
		case "WAM_Data":
			protoWriter = &wamProto
			protoNumber = wamDataProtoNum
			wamDataProtoNum++
			wamStruct.WriteString(fmt.Sprintf("    %s    %s    `json:\"%s\" sf:\"%s\"`\n", fName, sType, jName, rawSFName))

			if _, ok := map[string]struct{}{
				"Id":                     {},
				"Business_Currency__c":   {},
				"Business_Time_Zone__c":  {},
				"Form_Progress_Email__c": {},
				"Business_Industry__c":   {},
				"Business_Email__c":      {},
				"Location_Slug__c":       {},
			}[rawSFName]; !ok {
				wamCurl.WriteString(fmt.Sprintf("  \"%s\": %s,\n", rawSFName, curlVal))
			}
		case "Phone_Order":
			protoWriter = &noop
		case "Phone_Port":
			protoWriter = &noop
		case "N/A":
			protoWriter = &noop
		case "Sync_Integration":
			protoWriter = &noop
		case "Packages_Product":
			protoWriter = &noop
		default:
			return fmt.Errorf("unhandled sf object %s", sfObject)
		}
		protoWriter.WriteString(fmt.Sprintf("  %s %s = %d;\n", pType, pName, protoNumber))
	}

	wamProto.WriteString("}\n")
	wamStruct.WriteString("}\n")
	wamCurl.WriteString("}\n")

	err := os.Mkdir("output", os.ModePerm|os.ModeDir)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("error making directory: %s", err.Error())
	}

	for fName, w := range map[string]*strings.Builder{
		"wamProto":  &wamProto,
		"wamStruct": &wamStruct,
		"wamCurl":   &wamCurl,
	} {
		f, err := os.Create("output/" + fName)
		if err != nil {
			return fmt.Errorf("error opening file %s: %s", fName, err.Error())
		}
		defer f.Close()

		f.WriteString(w.String())
	}

	fmt.Println(wamCurl.String())

	return nil
}

type LineReader struct {
	lines []string
	index int
}

func NewLineReader(input string) *LineReader {
	return &LineReader{
		lines: strings.Split(input, "\n"),
	}
}

func (r *LineReader) skipFirstAndLast() {
	if r.index != 0 {
		panic("why would you do this?")
	}
	r.lines = r.lines[1 : len(r.lines)-1]
}

func (r *LineReader) nextSkipSpace() (string, bool) {
	for {
		l, ok := r.next()
		if !ok {
			return "", false
		}

		l = strings.TrimSpace(l)
		if l != "" {
			return l, true
		}
	}
}

func (r *LineReader) next() (string, bool) {
	if r.index >= len(r.lines) {
		return "", false
	}

	s := r.lines[r.index]
	r.index++
	return s, true
}

func snakeCaseToCamelCase(inputUnderScoreStr string) (camelCase string) {
	//snake_case to camelCase

	isToUpper := false

	for k, v := range inputUnderScoreStr {
		if k == 0 {
			camelCase = strings.ToLower(string(inputUnderScoreStr[0]))
		} else {
			if isToUpper {
				camelCase += strings.ToUpper(string(v))
				isToUpper = false
			} else {
				if v == '_' {
					isToUpper = true
				} else {
					camelCase += string(v)
				}
			}
		}
	}
	return

}

func rawData() string {
	dat, err := ioutil.ReadFile("input")
	if err != nil {
		panic(err)
	}
	return string(dat)
}
