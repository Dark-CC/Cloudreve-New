param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][string]$Sql
)

$ErrorActionPreference = "Stop"

$code = @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public class SqUtf8 {
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_open")] public static extern int Open(string f, out IntPtr db);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_prepare_v2")] public static extern int Prep(IntPtr db, string sql, int n, out IntPtr stmt, IntPtr tail);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_step")] public static extern int Step(IntPtr stmt);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_column_blob")] public static extern IntPtr ColBlob(IntPtr stmt, int i);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_column_bytes")] public static extern int ColBytes(IntPtr stmt, int i);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_column_type")] public static extern int ColType(IntPtr stmt, int i);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_column_count")] public static extern int ColCount(IntPtr stmt);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_finalize")] public static extern int Fin(IntPtr stmt);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_close")] public static extern int Close(IntPtr db);
  [DllImport("winsqlite3.dll", EntryPoint="sqlite3_errmsg")] public static extern IntPtr Err(IntPtr db);
  public static string Query(string file, string sql) {
    IntPtr db; var sb = new StringBuilder();
    if (Open(file, out db) != 0) return "OPEN FAIL";
    IntPtr st; int rc = Prep(db, sql, -1, out st, IntPtr.Zero);
    if (rc != 0) { sb.Append("PREP ERR: " + Marshal.PtrToStringAnsi(Err(db))); Close(db); return sb.ToString(); }
    while ((rc = Step(st)) == 100) {
      int n = ColCount(st);
      for (int i=0;i<n;i++) {
        if (ColType(st,i) == 5) {
          sb.Append("NULL");
        } else {
          IntPtr p = ColBlob(st,i);
          int len = ColBytes(st,i);
          byte[] bytes = new byte[len];
          if (len > 0) Marshal.Copy(p, bytes, 0, len);
          sb.Append(Encoding.UTF8.GetString(bytes));
        }
        if(i<n-1) sb.Append("|");
      }
      sb.Append("\n");
    }
    if (rc != 101) sb.Append("STEP ERR: " + Marshal.PtrToStringAnsi(Err(db)));
    Fin(st); Close(db); return sb.ToString();
  }
}
'@

Add-Type -TypeDefinition $code -Language CSharp
[SqUtf8]::Query((Resolve-Path -LiteralPath $Database).Path, $Sql)
