package detect

import (
	"debug/pe"
	"fmt"
	"os"
	"path/filepath"
)

func inspectDLL(directory PathFact, name, code string) DLLFact {
	fact := DLLFact{Name: name, Status: "unverified"}
	if directory.Path != "" {
		fact.Path = filepath.Join(directory.Path, name)
	}
	if directory.Status != "found" {
		fact.Issue = "an accessible ESAPI directory was not selected"
		return fact
	}
	path := inspectPath(fact.Path, directory.Source, false)
	fact.Status, fact.Issue = path.Status, path.Issue
	if path.Status != "found" {
		fact.Code = code + "_UNREADABLE"
		if path.Status == "missing" {
			fact.Code, fact.Issue = code+"_NOT_FOUND", name+" was not found"
		} else if path.Status == "invalid" {
			fact.Code = code + "_INVALID"
		}
		return fact
	}
	file, err := pe.Open(fact.Path)
	if err != nil {
		fact.Status, fact.Code, fact.Issue = "invalid", code+"_INVALID", err.Error()
		if os.IsPermission(err) {
			fact.Status, fact.Code = "unverified", code+"_UNREADABLE"
		}
		return fact
	}
	defer file.Close()
	info := &PEInfo{Machine: fmt.Sprintf("0x%04x", file.Machine), IsDLL: file.Characteristics&pe.IMAGE_FILE_DLL != 0}
	switch file.Machine {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		info.Machine = "amd64"
	case pe.IMAGE_FILE_MACHINE_I386:
		info.Machine = "i386"
	case pe.IMAGE_FILE_MACHINE_ARM64:
		info.Machine = "arm64"
	}
	// Only the CLR data-directory declaration is checked. Do not infer runtime
	// bitness from PE32/I386: a managed image can be AnyCPU. No DLL is loaded.
	switch header := file.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		info.Format = "PE32"
		info.HasCLRHeader = header.NumberOfRvaAndSizes > 14 && header.DataDirectory[14].VirtualAddress != 0 && header.DataDirectory[14].Size >= 72
	case *pe.OptionalHeader64:
		info.Format = "PE32+"
		info.HasCLRHeader = header.NumberOfRvaAndSizes > 14 && header.DataDirectory[14].VirtualAddress != 0 && header.DataDirectory[14].Size >= 72
	}
	if info.Format == "" {
		fact.Status, fact.Code, fact.Issue = "invalid", code+"_INVALID", "expected a PE32 or PE32+ optional header"
		return fact
	}
	fact.PE = info
	if !info.IsDLL || !info.HasCLRHeader {
		fact.Status, fact.Code, fact.Issue = "invalid", code+"_INVALID", "expected a PE DLL with a CLR header declaration"
	}
	return fact
}
