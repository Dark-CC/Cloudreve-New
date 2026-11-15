param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][string]$Sql
)

$ErrorActionPreference = "Stop"

$code = @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public class Sq {
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_open")] public static extern int Open(string f, out IntPtr db);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_prepare_v2")] public static extern int Prep(IntPtr db, string sql, int n, out IntPtr stmt, IntPtr tail);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_step")] public static extern int Step(IntPtr stmt);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_column_text")] public static extern IntPtr ColText(IntPtr stmt, int i);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_column_count")] public static extern int ColCount(IntPtr stmt);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_finalize")] public static extern int Fin(IntPtr stmt);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_close")] public static extern int Close(IntPtr db);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_errmsg")] public static extern IntPtr Err(IntPtr db);
  public static string Query(string file, string sql) {
    IntPtr db; var sb = new StringBuilder();
    if (Open(file, out db) != 0) return "OPEN FAIL";
    IntPtr st; int rc = Prep(db, sql, -1, out st, IntPtr.Zero);
    if (rc != 0) { sb.Append("PREP ERR: " + Marshal.PtrToStringAnsi(Err(db))); Close(db); return sb.ToString(); }
    while (Step(st) == 100) {
      int n = ColCount(st);
      for (int i=0;i<n;i++) { var p = ColText(st,i); sb.Append(p==IntPtr.Zero?"NULL":Marshal.PtrToStringAnsi(p)); if(i<n-1) sb.Append("|"); }
      sb.Append("\n");
    }
    Fin(st); Close(db); return sb.ToString();
  }
}
'@

Add-Type -TypeDefinition $code -Language CSharp
[Sq]::Query((Resolve-Path -LiteralPath $Database).Path, $Sql)
