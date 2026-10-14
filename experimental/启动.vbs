Set fso = CreateObject("Scripting.FileSystemObject")
base = fso.GetParentFolderName(WScript.ScriptFullName)
Set ws = CreateObject("WScript.Shell")
ws.CurrentDirectory = base
q = Chr(34)
py = "C:\Users\Wane\AppData\Local\Programs\Python\Python312\pythonw.exe"
cmd = q & py & q & " " & q & base & "\app.py" & q & " --show"
ws.Run cmd, 0, False
