package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// ExtractedInfo holds the extracted name and proto-type.
type ExtractedInfo struct {
	Name      string
	ProtoType string
}

// ExtractInfoFromFiles processes files matching the glob pattern and extracts information.
func ExtractInfoFromFiles(globPattern string) ([]ExtractedInfo, error) {
	var results []ExtractedInfo

	// Regular expression to match the desired lines and capture groups.
	re := regexp.MustCompile(`-- name: (\w+) (?:.*proto-type=(\w+\.\w+))?`)

	// Find files matching the glob pattern.
	files, err := filepath.Glob(globPattern)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			matches := re.FindStringSubmatch(line)
			if len(matches) > 0 {
				info := ExtractedInfo{
					Name:      matches[1],
					ProtoType: "",
				}
				if len(matches) > 2 && matches[2] != "" {
					info.ProtoType = matches[2]
				}
				results = append(results, info)
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}

	return results, nil
}

var (
	flagInputGlob  = flag.String("input", "models/*.sql", "Glob pattern for input files")
	flagModuleName = flag.String("module", "models", "Module name for generated code")
	flagOutputFile = flag.String("output", "/tmp/mapper.gen.go", "Output file name")
)

func main() {
	flag.Parse()

	globPattern := *flagInputGlob
	moduleName := *flagModuleName
	outputFilename := *flagOutputFile

	info, err := ExtractInfoFromFiles(globPattern)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// key is proto type, value is list of pggen structs
	pggenProtoMapping := make(map[string][]string)
	for _, i := range info {
		if i.ProtoType == "" {
			continue
		}
		if _, ok := pggenProtoMapping[i.ProtoType]; !ok {
			pggenProtoMapping[i.ProtoType] = make([]string, 0)
		}
		pggenProtoMapping[i.ProtoType] = append(pggenProtoMapping[i.ProtoType], i.Name)
	}

	for k, v := range pggenProtoMapping {
		fmt.Println(k, v)
	}

	cfg, err := readConfig("mapping.yaml")
	if err != nil {
		panic(err)
	}

	cfg.Mappings = []MappingItem{}
	sortedProtoTypes := []string{}
	for protoType := range pggenProtoMapping {
		sortedProtoTypes = append(sortedProtoTypes, protoType)
	}
	sort.Strings(sortedProtoTypes)

	for _, protoType := range sortedProtoTypes {
		pggenStructs := pggenProtoMapping[protoType]

		mapping := MappingItem{
			Name: "gen/protos/remote/upd88/com/comconnect/" + protoType,
		}
		for _, pggenStruct := range pggenStructs {
			mapping.From = append(mapping.From, moduleName+"."+pggenStruct+"Row")
		}
		cfg.Mappings = append(cfg.Mappings, mapping)
	}

	log.Printf("config: %+v\n", cfg)

	output, err := Generate(cfg)
	if err != nil {
		panic(err)
	}

	err = os.WriteFile(outputFilename, []byte(output), 0644)
	if err != nil {
		panic(err)
	}
}
