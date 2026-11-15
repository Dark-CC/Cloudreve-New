param([string]$DbPath,[string]$Email,[string]$Password)
$code = @"
using System;
using System.Security.Cryptography;
using System.Text;
public class PwTool {
  public static string Digest(string password) {
    var rnd = RandomNumberGenerator.Create();
    var bytes = new byte[16];
    rnd.GetBytes(bytes);
    var sb = new StringBuilder();
    foreach (var b in bytes) sb.Append((char)('a' + (b % 26)));
    var salt = sb.ToString();
    using (var sha = SHA256.Create()) {
      var hash = sha.ComputeHash(Encoding.UTF8.GetBytes(password + salt));
      var hs = new StringBuilder();
      foreach (var b in hash) hs.Append(b.ToString("x2"));
      return salt + ":" + hs.ToString();
    }
  }
}
"@
Add-Type -TypeDefinition $code -Language CSharp
$digest = [PwTool]::Digest($Password)
$sql = "UPDATE users SET password = '$digest' WHERE email = '$Email';"
& powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path (Split-Path $PSScriptRoot -Parent) 'tools/sqlite-exec.ps1') -Database $DbPath -Sql $sql
"$Email -> $digest"
