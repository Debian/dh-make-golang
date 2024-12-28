package main

import (
	"strings"
	"testing"
)

func TestReformatForControl(t *testing.T) {
	content := `
There are a few options for using a custom style:                       
                                                                        
 1. Call glamour.Render(inputText, "desiredStyle")                      
 2. Set the GLAMOUR_STYLE environment variable to your desired default  
 style or a file location for a style and call                          
 glamour.RenderWithEnvironmentConfig(inputText)                         
 3. Set the GLAMOUR_STYLE environment variable and pass                 
 glamour.WithEnvironmentConfig() to your custom renderer                
                                                                        
Check out these projects, which use glamour:                            
                                                                        
 * Glow (https://github.com/charmbracelet/glow), a markdown renderer for
 the command-line.                                                      
 * GitHub CLI (https://github.com/cli/cli), GitHub’s official command   
 line tool.                                                             
 * GLab (https://github.com/profclems/glab), An open source GitLab      
 command line tool.                                                     
`

	want := ` There are a few options for using a custom style:
 .
  1. Call glamour.Render(inputText, "desiredStyle")
  2. Set the GLAMOUR_STYLE environment variable to your desired default
     style or a file location for a style and call
     glamour.RenderWithEnvironmentConfig(inputText)
  3. Set the GLAMOUR_STYLE environment variable and pass
     glamour.WithEnvironmentConfig() to your custom renderer
 .
 Check out these projects, which use glamour:
 .
  * Glow (https://github.com/charmbracelet/glow), a markdown renderer for
    the command-line.
  * GitHub CLI (https://github.com/cli/cli), GitHub’s official command
    line tool.
  * GLab (https://github.com/profclems/glab), An open source GitLab
    command line tool.
`

	got := reformatForControl(content)
	if got != want {
		t.Errorf("\nwant\n====\n%v\ngot\n===\n%v", want, got)
	}
}

func TestMarkdownToLongDescription(t *testing.T) {
	content := `
## Styles

You can find all available default styles in our [gallery](https://github.com/charmbracelet/glamour/tree/master/styles/gallery).
Want to create your own style? [Learn how!](https://github.com/charmbracelet/glamour/tree/master/styles)

There are a few options for using a custom style:
1. Call §glamour.Render(inputText, "desiredStyle")§
1. Set the §GLAMOUR_STYLE§ environment variable to your desired default style or a file location for a style and call §glamour.RenderWithEnvironmentConfig(inputText)§
1. Set the §GLAMOUR_STYLE§ environment variable and pass §glamour.WithEnvironmentConfig()§ to your custom renderer


## Glamourous Projects

Check out these projects, which use §glamour§:
- [Glow](https://github.com/charmbracelet/glow), a markdown renderer for
the command-line.
- [GitHub CLI](https://github.com/cli/cli), GitHub’s official command line tool.
- [GLab](https://github.com/profclems/glab), An open source GitLab command line tool.
`
	content = strings.Replace(content, "§", "`", -1)

	want := ` Styles
 .
 You can find all available default styles in our gallery
 (https://github.com/charmbracelet/glamour/tree/master/styles/gallery).
 Want to create your own style? Learn how!
 (https://github.com/charmbracelet/glamour/tree/master/styles)
 .
 There are a few options for using a custom style:
 .
  1. Call glamour.Render(inputText, "desiredStyle")
  2. Set the GLAMOUR_STYLE environment variable to your desired default
     style or a file location for a style and call glamour.
     RenderWithEnvironmentConfig(inputText)
  3. Set the GLAMOUR_STYLE environment variable and pass glamour.
     WithEnvironmentConfig() to your custom renderer
 .
 Glamourous Projects
 .
 Check out these projects, which use glamour:
 .
  * Glow (https://github.com/charmbracelet/glow), a markdown renderer for
    the command-line.
  * GitHub CLI (https://github.com/cli/cli), GitHub’s official command
    line tool.
  * GLab (https://github.com/profclems/glab), An open source GitLab
    command line tool.
`

	got, err := markdownToLongDescription(content)
	if err != nil {
		t.Errorf("markdownToLongDescription failed: %v", err)
	}
	if got != want {
		t.Errorf("\nwant\n====\n%v\ngot\n===\n%v", want, got)
	}
}
