package docs

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
)

const FrontMatter = `---

layout: single
type: docs
title: "Documentation | {{ .Description }} | {{ $name }}"

---`

type Entry struct {
	Title          string
	Template       *template.Template
	Specifications []*Entry
	Examples       []*Entry
	Description    string
	FileName       string
	CmdEntryPoint  string
	URL            string
	BaseURL        string
}

func GenerateFileName(dir, s string) string {
	return (dir + strings.Replace(strings.ToLower(s), " ", "_", -1) + ".md")
}

func GenerateTitleFromFileName(file string) string {
	file = strings.Replace(file, "_", " ", -1)
	file = strings.Replace(file, "-", " ", -1)
	return strings.Title(strings.Replace(file, ".md", "", 1))
}

func GenerateFileNameFromGlob(dir, s string) string {
	return (dir + strings.Replace(filepath.Base(s), " ", "_", -1))
}

func GenerateURLFromFileName(s string) string {
	s = strings.Replace(s, "./", "/", 1)
	return strings.Replace(s, ".md", "/", -1)
}

func GenerateCommandsTemplate() (*template.Template, error) {
	handle_link := func(s string) string {
		return (strings.Replace(s, ".md", "/", -1))
	}

	handle_file := func(s1, s2 string) string {
		if s2 == "" {
			return strings.Replace((s1 + ".md"), " ", "_", -1)
		}
		return strings.Replace((s1 + " " + s2 + ".md"), " ", "_", -1)
	}

	funcMap := template.FuncMap{
		"title":       strings.Title,
		"replace":     strings.Replace,
		"chomp":       strings.TrimSpace,
		"handle_file": handle_file,
		"handle_link": handle_link,
	}

	var templateText = `{{- $name := .Command.CommandPath -}}` + FrontMatter + `

# {{ $name }}

{{ title .Command.Short }}

{{ if .Command.Runnable }}## Usage

` + "```bash\n{{ .Command.UseLine }}\n```" + `{{ end }}

{{ if ne .Command.Long  "" }}## Synopsis

{{ .Command.Long }}
{{ end }}
{{ $flags := .Command.NonInheritedFlags }}
{{ if $flags.HasFlags }}## Options

` + "```bash\n  {{ $flags.FlagUsages | chomp }}\n```" + `{{ end }}
{{ $globalFlags := .Command.InheritedFlags }}
{{ if $globalFlags.HasFlags }}## Options inherited from parent commands

` + "```bash\n  {{ $globalFlags.FlagUsages | chomp }}\n```" + `{{ end }}

{{ if .Command.HasSubCommands }}# Subcommands
{{ range .Command.Commands }}
* [{{ $name }} {{ .Name }}]({{ $.Entry.BaseURL }}{{ handle_file $name .Name | handle_link }}) - {{ .Short }}
{{- end }}
{{ end }}

{{ if .Command.HasParent }}{{ $parent := .Command.Parent }}## See Also

* [{{ $parent.CommandPath }}]({{ .Entry.BaseURL }}{{ handle_file $parent.CommandPath "" | handle_link }}) - {{ $parent.Short }}
{{ end }}

{{ if ne .Command.Example "" }}# Quick Tips

` + "```bash\n{{ .Command.Example }}\n```" + `{{ end }}

{{ if ne (len .Entry.Examples) 0 }}# Examples
{{ range .Entry.Examples }}
* [{{ title .Title }}]({{ .URL }})
{{- end }}
{{ end }}

{{ if ne (len .Entry.Specifications) 0 }}# Specifications
{{ range .Entry.Specifications }}
* [{{ title .Title }}]({{ .URL }})
{{- end }}
{{ end }}
`

	return template.New("docGenerator").Funcs(funcMap).Parse(templateText)
}

func GenerateEntries(dir, renderDir, description string) ([]*Entry, error) {
	var entries []*Entry

	if _, err := os.Stat(renderDir); os.IsNotExist(err) {
		err = os.MkdirAll(renderDir, 0755)
		if err != nil {
			panic(err)
		}
	}

	files := CollectEntries(dir)

	for _, file := range files {
		thisEntry, err := GenerateEntry(file, dir, renderDir, description)
		if err != nil {
			return nil, err
		} else {
			entries = append(entries, thisEntry)
		}
	}

	return entries, nil
}

func CollectEntries(dir string) []string {
	var newFiles []string

	files, err := filepath.Glob(dir + "/*")
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		fileInfo, err := os.Stat(file)

		if err != nil {
			panic(err)
		}

		if fileInfo.IsDir() {
			newFiles = append(newFiles, CollectEntries(file)...)
		} else {
			if filepath.Ext(file) == ".md" {
				newFiles = append(newFiles, file)
			}
		}
	}

	return newFiles
}

func GenerateEntry(file, dir, renderDir, description string) (*Entry, error) {
	var err error

	thisEntry := &Entry{
		FileName:    GenerateFileNameFromGlob(renderDir, file),
		Title:       GenerateTitleFromFileName(filepath.Base(file)),
		Description: description,
	}

	thisEntry.URL = GenerateURLFromFileName(thisEntry.FileName)

	txt, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	// Get template from docs generator.
	thisEntry.Template, err = GenerateEntriesTemplate(txt)
	if err != nil {
		return nil, err
	}

	return thisEntry, nil
}

func GenerateEntriesTemplate(txt []byte) (*template.Template, error) {
	handle_link := func(s string) string {
		return (strings.Replace(s, ".md", "/", -1))
	}

	handle_file := func(s1 string) string {
		return strings.Replace((s1 + ".md"), " ", "_", -1)
	}

	insert_definition := func(file string, struc string) string {
		txt, err := ioutil.ReadFile(filepath.Join("definitions", file))
		if err != nil {
			panic(err)
		}
		finder := regexp.MustCompile(fmt.Sprintf(`(?ms:^type %s struct {.*?^})`, struc))
		return ("```go\n" + string(finder.Find(txt)) + "\n```")
	}

	insert_bash_lines := func(file string, linesToRead string) string {
		var lines []byte
		var line []byte
		var start int
		var stop int

		fileInfo, err := os.Open(filepath.Join("docs", "tests", file))
		if err != nil {
			panic(err)
		}
		defer fileInfo.Close()

		start, err = strconv.Atoi(strings.Split(linesToRead, "-")[0])
		if strings.Contains(linesToRead, "-") {
			stop, err = strconv.Atoi(strings.Split(linesToRead, "-")[1])
		} else {
			stop = start
		}
		if err != nil {
			panic(err)
		}

		r := bufio.NewReader(fileInfo)
		for i := 1; ; i++ {
			line, err = r.ReadBytes('\n')
			if err != nil {
				break
			}
			if i >= start && i <= stop {
				lines = append(lines, line...)
			}
		}
		if err != io.EOF {
			panic(err)
		}

		return ("```bash\n" + string(lines) + "```")
	}

	insert_file := func(file string) string {
		file = filepath.Join("docs", "tests", file)
		ext := filepath.Ext(file)
		switch ext {
		case ".sol":
			ext = ".javascript"
		case ".yml":
			ext = ".yaml"
		}

		ext = strings.Replace(ext, ".", "", 1)

		txtB, err := ioutil.ReadFile(file)
		if err != nil {
			panic(err)
		}

		txt := string(txtB)
		if !strings.HasSuffix(txt, "\n") {
			txt = txt + "\n"
		}

		return ("```" + ext + "\n" + txt + "```") // TODO: add auto-curl text
	}

	funcMap := template.FuncMap{
		"title":             strings.Title,
		"replace":           strings.Replace,
		"chomp":             strings.TrimSpace,
		"handle_file":       handle_file,
		"handle_link":       handle_link,
		"insert_definition": insert_definition,
		"insert_bash_lines": insert_bash_lines,
		"insert_file":       insert_file,
	}

	var templateText = `{{- $name := .Title -}}` + FrontMatter + `

` + string(txt) + `

## Commands

* [{{ .CmdEntryPoint }}]({{ .BaseURL }}{{ handle_file .CmdEntryPoint | handle_link }})

{{ if ne (len .Examples) 0 }}# Examples
{{ range .Examples }}
* [{{ title .Title }}]({{ .URL }})
{{- end }}
{{ end }}

{{ if ne (len .Specifications) 0 }}# Specifications
{{ range .Specifications }}
* [{{ title .Title }}]({{ .URL }})
{{- end }}
{{ end }}
`

	return template.New("entryGenerator").Funcs(funcMap).Parse(templateText)
}

func RenderEntry(thisEntry *Entry) error {
	outFile, err := os.Create(thisEntry.FileName)
	if err != nil {
		return err
	}
	defer outFile.Close()

	err = thisEntry.Template.Execute(outFile, thisEntry)
	if err != nil {
		return err
	}

	return nil
}
