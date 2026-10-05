param([Parameter(Mandatory=$true)][string]$Command)

# This is a deliberately limited evaluation tool, not a general-purpose sandbox.
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseInput($Command, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -gt 0) { throw 'EVAL_POLICY: invalid PowerShell syntax' }
$allowed = @{
  'get-childitem' = @('path','literalpath','recurse','file','force','filter','include','exclude','name','erroraction')
  'get-content' = @('path','literalpath','totalcount','raw','encoding','erroraction')
  'get-command' = @('name','erroraction')
  'select-string' = @('path','literalpath','pattern','simplematch','casesensitive','list','erroraction')
  'select-object' = @('property','first','skip','expandproperty')
  'format-list' = @('property')
  'convertto-json' = @('depth','compress')
  'rg' = @('n','i','l','f','s','files','hidden','glob','g')
}
$forbiddenTypes = @(
  'VariableExpressionAst','ExpandableStringExpressionAst','SubExpressionAst',
  'ScriptBlockExpressionAst','AssignmentStatementAst','InvokeMemberExpressionAst',
  'MemberExpressionAst','TypeExpressionAst','RedirectionAst','FileRedirectionAst','MergingRedirectionAst','CommandExpressionAst',
  'BinaryExpressionAst','UnaryExpressionAst','UsingExpressionAst','FunctionDefinitionAst',
  'IfStatementAst','ForEachStatementAst','WhileStatementAst','TryStatementAst'
)
$badNodes = $ast.FindAll({ param($node) $forbiddenTypes -contains $node.GetType().Name }, $true)
if ($badNodes.Count -gt 0) { throw 'EVAL_POLICY: only literal read-only command pipelines are supported' }
foreach ($cmd in $ast.FindAll({ param($node) $node -is [System.Management.Automation.Language.CommandAst] }, $true)) {
  $name = $cmd.GetCommandName()
  if (-not $name -or -not $allowed.ContainsKey($name.ToLowerInvariant()) -or $cmd.InvocationOperator -ne 'Unknown') {
    throw 'EVAL_POLICY: unsupported executable or invocation'
  }
  foreach ($element in $cmd.CommandElements) {
    if ($element -is [System.Management.Automation.Language.CommandParameterAst]) {
      if ($allowed[$name.ToLowerInvariant()] -notcontains $element.ParameterName.ToLowerInvariant().TrimStart('-')) {
        throw 'EVAL_POLICY: unsupported parameter'
      }
    }
  }
}
foreach ($literal in $ast.FindAll({ param($node) $node -is [System.Management.Automation.Language.StringConstantExpressionAst] }, $true)) {
  if ($literal.Value -match '(^|[\\/])\.\.([\\/]|$)|^[\\/]|^[a-zA-Z]:|::') {
    throw 'EVAL_POLICY: paths must stay relative to the fixture'
  }
}
$ErrorActionPreference = 'Stop'
& ([scriptblock]::Create($Command))
