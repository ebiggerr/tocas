package document

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/TeaMeow/TocasUI/src/CLI/executor"
	"github.com/TeaMeow/TocasUI/src/CLI/path"
	"github.com/alecthomas/template"

	"github.com/TeaMeow/TocasUI/src/CLI/util"
	blackfriday "gopkg.in/russross/blackfriday.v2"
	yaml "gopkg.in/yaml.v2"
)

func Compile(path string) {
	d := New(path)
	// 讀取文件的內容。
	log.Printf("Read")
	d.Read()
	// 將文件內容的 YAML 資料轉譯並映射到本地結構體。
	log.Printf("Unmarshal")
	d.Unmarshal()
	//
	log.Printf("Analyze")
	d.Analyze()
	//
	log.Printf("Anchor")
	d.Anchor()
	//
	log.Printf("Markdown")
	d.Markdown()
	// 將部份內容轉換成預置模板標籤，避免被 Highlight JS 轉譯。
	log.Printf("Placeholder")
	d.Placeholder()
	//
	log.Printf("Beatify")
	d.Beatify()
	//
	log.Printf("Trim")
	d.Trim()
	// 將範例程式碼透過 Highlight JS 螢光標記。
	log.Printf("Highlight")
	d.Highlight()
	// 將原先的預置模板標籤轉換回真正的 HTML 程式碼。
	log.Printf("Tag")
	d.Tag()
	//
	log.Printf("Clean")
	d.Clean()
	// 索引所有章節。
	log.Printf("Index")
	d.Index()
	//
	log.Printf("LoadUI")
	d.LoadUI()
	// 將文件透過模板引擎編譯成一個靜態的網頁內容。
	log.Printf("Compile")
	d.Compile()
	// 將靜態網頁保存於指定位置。
	log.Printf("Save")
	d.Save()
}

// 美化
// Code 裡面的 IMAGE PATH
//

func (d *Document) LoadUI() {
	b, err := ioutil.ReadFile(fmt.Sprintf("%s%s/%s.yml", path.TranslationsPath, d.Language, d.Language))
	if err != nil {
		panic(err)
	}
	err = yaml.Unmarshal(b, &d.UI)
	if err != nil {
		panic(err)
	}
	var linkContainer *LinkContainer
	err = yaml.Unmarshal(b, &linkContainer)
	if err != nil {
		panic(err)
	}
	var contributorContainer *ContributorContainer
	err = yaml.Unmarshal(b, &contributorContainer)
	if err != nil {
		panic(err)
	}
	d.Links = linkContainer.Links
	d.Contributors = contributorContainer.Contributors

	for _, v := range d.Links {
		for _, i := range v.Items {
			i.URL = fmt.Sprintf("/components/%s.html", ToAnchor(i.Alias))
			i.IsCurrent = d.Name == ToAnchor(i.Alias)
		}
	}
}

func (d *Document) Clean() {
	replace := func(input string) string {
		input = regexp.MustCompile(`\[\[(.*?)\]\]`).ReplaceAllString(input, "$1")
		input = regexp.MustCompile(`{{(.*?)}}`).ReplaceAllString(input, "$1")
		input = util.ReplaceAllStringSubmatchFunc(regexp.MustCompile(`!-(.*?)-!`), input, func(groups []string) string {
			if len(groups) == 0 {
				return ""
			}
			return d.Asset(groups[1], true)
		})
		return input
	}
	for _, v := range d.Definitions {
		for _, s := range v.Sections {
			if s.HTML != "" {
				s.HTMLNative = replace(s.HTML)
			}
			if s.CSS != "" {
				s.CSSNative = replace(s.CSS)
			}
			if s.JavaScript != "" {
				s.JavaScriptNative = replace(s.JavaScript)
			}
		}
	}
}

func (d *Document) Markdown() {
	d.Description = string(blackfriday.Run([]byte(d.Description)))
	d.Outline = string(blackfriday.Run([]byte(d.Outline)))
	for _, v := range d.Definitions {
		v.Description = string(blackfriday.Run([]byte(v.Description)))
		for _, s := range v.Sections {
			s.Description = string(blackfriday.Run([]byte(s.Description)))
		}
	}
}

func (d *Document) Anchor() {
	for _, v := range d.Definitions {
		for _, s := range v.Sections {
			s.Anchor = ToAnchor(s.Title)
		}
	}
}

func (d *Document) Trim() {
	replace := func(input string, cutsets []string) string {
		for _, v := range cutsets {
			input = strings.Replace(input, v, "", -1)
		}
		return input
	}
	for _, v := range d.Definitions {
		for _, s := range v.Sections {
			if s.HTML != "" {
				s.HTMLReadable = replace(s.HTMLReadable, s.Remove)
			}
			if s.CSS != "" {
				s.CSSReadable = replace(s.CSSReadable, s.Remove)
			}
			if s.JavaScript != "" {
				s.JavaScriptReadable = replace(s.JavaScriptReadable, s.Remove)
			}
		}
	}
}

func (d *Document) Beatify() {
	replace := func(input string, typ string) string {
		var cmd *exec.Cmd
		switch typ {
		case "css":
			cmd = exec.Command("js-beautify", "--css", "-L false", "-N false")
		case "js":
			cmd = exec.Command("js-beautify")
		case "html":
			cmd = exec.Command("js-beautify", "--html")
		}
		cmd.Stdin = bytes.NewBuffer([]byte(input))
		output, err := cmd.CombinedOutput()
		if err != nil {
			panic(err)
		}
		return string(output)
	}

	var waitGroup sync.WaitGroup

	for _, v := range d.Definitions {
		for _, s := range v.Sections {
			waitGroup.Add(1)
			go func(s *DefinitionSection) {
				if s.HTML != "" {
					s.HTMLReadable = replace(s.HTMLReadable, "html")
				}
				if s.CSS != "" {
					s.CSSReadable = replace(s.CSSReadable, "css")
				}
				if s.JavaScript != "" {
					s.JavaScriptReadable = replace(s.JavaScriptReadable, "js")
				}
				waitGroup.Done()
			}(s)
		}
	}
	waitGroup.Wait()
}

// New 會讀取一個指定路徑中的 YAML 文件資料，
// 並且以此作為基礎建立一個新的文件結構體。
func New(fullpath string) *Document {
	//
	parts := strings.Split(fullpath, "/translations/")
	//
	parts = strings.Split(parts[1], "/")
	//
	basename := filepath.Base(fullpath)
	//
	return &Document{
		Language:        parts[0],
		Name:            strings.TrimSuffix(basename, filepath.Ext(basename)),
		Path:            fullpath,
		CompiledContent: &DocumentContent{},
		Executor:        executor.New(),
		Indexes:         &DocumentIndexes{},
		UI:              make(map[interface{}]interface{}),
	}
}

// Read 會載入文件的內容，並且讀入到結構體中。
func (d *Document) Read() {
	b, err := ioutil.ReadFile(d.Path)
	if err != nil {
		panic(err)
	}
	d.Content = b
}

// Unmarshal 會將讀入的資料透過 YAML 解析並映射到本地的文件欄位來供程式使用。
func (d *Document) Unmarshal() {
	err := yaml.Unmarshal(d.Content, &d)
	if err != nil {
		panic(err)
	}
	//
	if d.Settings != nil {
		d.HasSettings = true
	}
	if d.Usages != nil {
		d.HasUsages = true
	}
	if len(d.Definitions) > 0 {
		d.HasDefinitions = true
	}
}

//
func (d *Document) Analyze() {
	replace := func(input string) []string {
		var output []string
		result := regexp.MustCompile(`\[\[(.*?)\]\]`).FindAllStringSubmatch(input, -1)
		for _, v := range result {
			var has bool
			for _, vv := range output {
				if vv == v[1] {
					has = true
					break
				}
			}
			if !has {
				output = append(output, v[1])
			}
		}
		return output
	}
	for _, v := range d.Definitions {
		for _, s := range v.Sections {
			if s.HTML != "" {
				s.Highlights = replace(s.HTML)
			}
			if s.CSS != "" {
				s.Highlights = replace(s.CSS)
			}
			if s.JavaScript != "" {
				s.Highlights = replace(s.JavaScript)
			}
		}
	}
}

//
func (d *Document) Escape() {

}

// Placeholder 會將指定模板符號轉換成更獨特的預置符號避免解析時出錯，
// 在那之後會由另一個程序將預置符號轉換回正常的文字。
func (d *Document) Placeholder() {
	replace := func(input string) string {
		input = regexp.MustCompile(`\[\[(.*?)\]\]`).ReplaceAllString(input, `MARK${1}MARKEND`)
		input = regexp.MustCompile(`{{(.*?)}}`).ReplaceAllString(input, `COMP${1}COMPEND`)
		input = regexp.MustCompile(`!-(.*?)-!`).ReplaceAllString(input, `IMAG${1}IMAGEND`)
		return input
	}
	for _, v := range d.Definitions {
		for _, s := range v.Sections {
			if s.HTML != "" {
				s.HTMLReadable = replace(s.HTML)
			}
			if s.CSS != "" {
				s.CSSReadable = replace(s.CSS)
			}
			if s.JavaScript != "" {
				s.JavaScriptReadable = replace(s.JavaScript)
			}
		}
	}
}

// Asset 會接收一個檔案的簡寫名稱，並且將其轉換成真正指向到 Tocas 文件靜態圖片、資源檔案的路徑。
func (d *Document) Asset(name string, real bool) string {
	if !real {
		switch name {
		case "16:9":
		case "1:1":
		case "4:3":
			return "image.png"
		case "user":
		case "user2":
		case "user3":
			return "user.png"
		case "embed:karen":
		case "embed:vimeo":
			return "placeholder.png"
		case "embed:video":
			return "video.mp4"
		}
	}
	switch name {
	case "16:9":
		name = "images/16-9.png"
	case "1:1":
		name = "images/1-1.png"
	case "4:3":
		name = "images/4-3.png"
	case "user":
		name = "images/user.png"
	case "user2":
		name = "images/user2.png"
	case "user3":
		name = "images/user3.png"
	case "embed:karen":
		name = "images/videos/karen.png"
	case "embed:vimeo":
		name = "images/videos/vimeo.png"
	case "embed:video":
		name = "videos/video.mp4"
	}
	return fmt.Sprintf("%s%s", path.AssetsPath, name)
}

// Component 會接收一個元件的簡寫，並且將其轉換成指向到該文件的連結路徑。
func (d *Document) Component(name string) string {
	return fmt.Sprintf("<a href='/components/%s.html'>%s</a>", name, name)
}

// Tag 會分析接收到的內容，並且將其中的文件標籤符號轉換成真正的內容和程式碼。
func (d *Document) Tag() {
	replace := func(input string) string {
		// 將螢光標記模板符號轉換回 `<mark>` 的 HTML 標記程式碼。
		input = regexp.MustCompile(`MARK(.*?)MARKEND`).ReplaceAllString(input, "<mark>$1</mark>")
		// 將圖片模板符號轉換成真正的預置圖片路徑。
		input = util.ReplaceAllStringSubmatchFunc(regexp.MustCompile(`IMAG(.*?)IMAGEND`), input, func(groups []string) string {
			if len(groups) == 0 {
				return ""
			}
			return d.Asset(groups[1], false)
		})
		// 將元件連結模板符號轉換成真正指向到該元件文件的 HTML 連結程式碼。
		input = util.ReplaceAllStringSubmatchFunc(regexp.MustCompile(`COMP(.*?)COMPEND`), input, func(groups []string) string {
			if len(groups) == 0 {
				return ""
			}
			return d.Component(groups[1])
		})
		return input
	}
	for _, v := range d.Definitions {
		for _, s := range v.Sections {
			if s.HTML != "" {
				s.HTMLReadable = replace(s.HTMLReadable)
			}
			if s.CSS != "" {
				s.CSSReadable = replace(s.CSSReadable)
			}
			if s.JavaScript != "" {
				s.JavaScriptReadable = replace(s.JavaScriptReadable)
			}
		}
	}
}

// Index 能夠索引文件的所有段落，並且將其彙整（這個函式必須在整體文件分析完畢後才能夠執行）。
func (d *Document) Index() {
	for _, v := range d.Definitions {
		i := &DocumentIndex{
			Title: v.Title,
			Name:  ToAnchor(v.Title),
		}
		for _, s := range v.Sections {
			i.SubIndexes = append(i.SubIndexes, &DocumentIndex{
				Title:  s.Title,
				Name:   ToAnchor(s.Title),
				Labels: s.Highlights,
			})
		}
		i.HasSubIndex = len(i.SubIndexes) > 0
		d.Indexes.DefinitionIndexes = append(d.Indexes.DefinitionIndexes, i)
	}
}

// ToAnchor 可以將傳入的標題字串轉換成更符合連結錨點的名稱，通常是將空白轉換成減號分隔符號。
func ToAnchor(input string) string {
	return strings.ToLower(strings.Replace(input, " ", "-", -1))
}

//
func (d *Document) Highlight() {
	replace := func(input string) string {
		cmd := exec.Command("hljs", "-l", "html")
		cmd.Stdin = bytes.NewBuffer([]byte(input))
		output, err := cmd.CombinedOutput()
		if err != nil {
			panic(err.Error() + string(output))
		}
		return string(output)
	}

	var waitGroup sync.WaitGroup

	for _, v := range d.Definitions {
		for _, s := range v.Sections {
			waitGroup.Add(1)
			go func(s *DefinitionSection) {
				if s.HTML != "" {
					s.HTMLReadable = replace(s.HTMLReadable)
				}
				if s.CSS != "" {
					s.CSSReadable = replace(s.CSSReadable)
				}
				if s.JavaScript != "" {
					s.JavaScriptReadable = replace(s.JavaScriptReadable)
				}
				waitGroup.Done()
			}(s)
		}
	}
	waitGroup.Wait()
}

func (d *Document) UIString(input string) string {
	keys := strings.Split(input, ".")
	currentMap := d.UI

	for i, v := range keys {
		if i+1 == len(keys) {
			break
		}
		currentMap = currentMap[v].(map[interface{}]interface{})
	}
	//fmt.Printf("%+v", input)
	input = currentMap[keys[len(keys)-1]].(string)

	if strings.Contains(input, "[") || strings.Contains(input, "*") || strings.Contains(input, "`") {
		input = string(blackfriday.Run([]byte(input)))
	}

	return input
}

//
func (d *Document) Compile() {
	parse := func(ppath string, data map[string]interface{}) string {
		tmpl := template.New(filepath.Base(ppath))
		tmpl.Funcs(template.FuncMap{
			"String": d.UIString,
		})
		tmpl, err := tmpl.ParseFiles(ppath)
		if err != nil {
			panic(err)
		}
		buf := bytes.NewBuffer([]byte(""))
		err = tmpl.Execute(buf, data)
		//if err != nil {
		//	panic(err)
		//}
		return string(buf.Bytes())
	}

	compile := func(ppath string) string {
		data := map[string]interface{}{
			"Document": d,
			"UI":       d.UI,
		}
		sub := parse(ppath, data)
		return parse(fmt.Sprintf("%ssingle.html", path.TemplateViewsPath), map[string]interface{}{
			"Document": d,
			"UI":       d.UI,
			"HTML":     sub,
		})
	}

	d.CompiledContent.Definitions = compile(fmt.Sprintf("%sdefinitions.html", path.TemplateComponentsPath))
	d.CompiledContent.Settings = compile(fmt.Sprintf("%ssettings.html", path.TemplateComponentsPath))
	d.CompiledContent.Usages = compile(fmt.Sprintf("%susages.html", path.TemplateComponentsPath))
}

// Save 能夠將整個已經解析且編譯完的文件作為靜態檔案而保存到指定的位置。
func (d *Document) Save() {
	save := func(path string, content string) {
		if content != "" {

			if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
				err = os.MkdirAll(filepath.Dir(path), 0777)
				if err != nil {
					panic(err)
				}
			}

			err := ioutil.WriteFile(path, []byte(content), 0777)
			if err != nil {
				panic(err)
			}
		}
	}

	// 依照讀取檔案決定編譯內容
	save(fmt.Sprintf("%s%s/components/%s.html", path.DocsDistPath, d.Language, d.Name), d.CompiledContent.Definitions)
	save(fmt.Sprintf("%s%s/components/%s-settings.html", path.DocsDistPath, d.Language, d.Name), d.CompiledContent.Settings)
	save(fmt.Sprintf("%s%s/components/%s-usages.html", path.DocsDistPath, d.Language, d.Name), d.CompiledContent.Usages)
}
