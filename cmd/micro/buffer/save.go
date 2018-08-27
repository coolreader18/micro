package buffer

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	. "github.com/zyedidia/micro/cmd/micro/util"

	"github.com/zyedidia/micro/cmd/micro/config"
)

// Save saves the buffer to its default path
func (b *Buffer) Save() error {
	return b.SaveAs(b.Path)
}

// SaveAs saves the buffer to a specified path (filename), creating the file if it does not exist
func (b *Buffer) SaveAs(filename string) error {
	// TODO: rmtrailingws and updaterules
	b.UpdateRules()
	// if b.Settings["rmtrailingws"].(bool) {
	// 	for i, l := range b.lines {
	// 		pos := len(bytes.TrimRightFunc(l.data, unicode.IsSpace))
	//
	// 		if pos < len(l.data) {
	// 			b.deleteToEnd(Loc{pos, i})
	// 		}
	// 	}
	//
	// 	b.Cursor.Relocate()
	// }

	if b.Settings["eofnewline"].(bool) {
		end := b.End()
		if b.RuneAt(Loc{end.X - 1, end.Y}) != '\n' {
			b.Insert(end, "\n")
		}
	}

	// Update the last time this file was updated after saving
	defer func() {
		b.ModTime, _ = GetModTime(filename)
	}()

	// Removes any tilde and replaces with the absolute path to home
	absFilename, _ := ReplaceHome(filename)

	// TODO: save creates parent dirs
	// // Get the leading path to the file | "." is returned if there's no leading path provided
	// if dirname := filepath.Dir(absFilename); dirname != "." {
	// 	// Check if the parent dirs don't exist
	// 	if _, statErr := os.Stat(dirname); os.IsNotExist(statErr) {
	// 		// Prompt to make sure they want to create the dirs that are missing
	// 		if yes, canceled := messenger.YesNoPrompt("Parent folders \"" + dirname + "\" do not exist. Create them? (y,n)"); yes && !canceled {
	// 			// Create all leading dir(s) since they don't exist
	// 			if mkdirallErr := os.MkdirAll(dirname, os.ModePerm); mkdirallErr != nil {
	// 				// If there was an error creating the dirs
	// 				return mkdirallErr
	// 			}
	// 		} else {
	// 			// If they canceled the creation of leading dirs
	// 			return errors.New("Save aborted")
	// 		}
	// 	}
	// }

	var fileSize int

	err := overwriteFile(absFilename, func(file io.Writer) (e error) {
		if len(b.lines) == 0 {
			return
		}

		// end of line
		var eol []byte
		if b.Settings["fileformat"] == "dos" {
			eol = []byte{'\r', '\n'}
		} else {
			eol = []byte{'\n'}
		}

		// write lines
		if fileSize, e = file.Write(b.lines[0].data); e != nil {
			return
		}

		for _, l := range b.lines[1:] {
			if _, e = file.Write(eol); e != nil {
				return
			}
			if _, e = file.Write(l.data); e != nil {
				return
			}
			fileSize += len(eol) + len(l.data)
		}
		return
	})

	if err != nil {
		return err
	}

	if !b.Settings["fastdirty"].(bool) {
		if fileSize > LargeFileThreshold {
			// For large files 'fastdirty' needs to be on
			b.Settings["fastdirty"] = true
		} else {
			calcHash(b, &b.origHash)
		}
	}

	b.Path = filename
	absPath, _ := filepath.Abs(filename)
	b.AbsPath = absPath
	b.isModified = false
	return b.Serialize()
}

// SaveWithSudo saves the buffer to the default path with sudo
func (b *Buffer) SaveWithSudo() error {
	return b.SaveAsWithSudo(b.Path)
}

// SaveAsWithSudo is the same as SaveAs except it uses a neat trick
// with tee to use sudo so the user doesn't have to reopen micro with sudo
func (b *Buffer) SaveAsWithSudo(filename string) error {
	b.UpdateRules()
	b.Path = filename
	absPath, _ := filepath.Abs(filename)
	b.AbsPath = absPath

	// Set up everything for the command
	cmd := exec.Command(config.GlobalSettings["sucmd"].(string), "tee", filename)
	cmd.Stdin = bytes.NewBuffer(b.Bytes())

	// This is a trap for Ctrl-C so that it doesn't kill micro
	// Instead we trap Ctrl-C to kill the program we're running
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			cmd.Process.Kill()
		}
	}()

	// Start the command
	cmd.Start()
	err := cmd.Wait()

	if err == nil {
		b.isModified = false
		b.ModTime, _ = GetModTime(filename)
		return b.Serialize()
	}
	return err
}
