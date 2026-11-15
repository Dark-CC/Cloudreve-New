param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][string]$Sql
)

$ErrorActionPreference = "Stop"

$code = @'
using System;
using System.Runtime.InteropServices;
public class SqExec {
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_open")] public static extern int Open(string f, out IntPtr db);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_exec")] public static extern int Exec(IntPtr db, string sql, IntPtr cb, IntPtr arg, out IntPtr err);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_close")] public static extern int Close(IntPtr db);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_free")] public static extern void Free(IntPtr p);
  public static int Run(string file, string sql, out string error) {
    IntPtr db; error = "";
    if (Open(file, out db) != 0) { error = "OPEN FAIL"; return -1; }
    IntPtr msg;
    int rc = Exec(db, sql, IntPtr.Zero, IntPtr.Zero, out msg);
    if (rc != 0) { error = Marshal.PtrToStringAnsi(msg); Free(msg); }
    Close(db);
    return rc;
  }
}
'@

Add-Type -TypeDefinition $code -Language CSharp
$errorMessage = ""
$result = [SqExec]::Run((Resolve-Path -LiteralPath $Database).Path, $Sql, [ref]$errorMessage)
if ($result -ne 0) {
    throw "SQLite error: $errorMessage"
}
