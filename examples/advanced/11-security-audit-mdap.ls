# Enterprise Security Audit MDAP Pipeline
# ========================================
#
# This example demonstrates MDAP (Massively Decomposed Agentic Processes)
# for a critical software engineering task: scanning thousands of code
# locations for complex security vulnerabilities.
#
# Task: Identify and remediate SQL injection and XSS patterns in a legacy PHP/JS codebase.
#
# Why MDAP?
# - Security audits require extreme precision (near-zero false negatives).
# - Traditional static analysis often has high false positives.
# - Voting between multiple specialized LLM samples increases detection accuracy.

# =============================================================================
# Agent Definitions
# =============================================================================

agent "security-auditor" {
    model: "claude-sonnet-4-20250514"
    instruction: ```
You are a senior security researcher. Analyze the provided code for:
1. SQL Injection (unparameterized queries)
2. Cross-Site Scripting (XSS) (unescaped output)
3. Broken Access Control

For each finding, provide:
- Severity (Critical/High/Medium/Low)
- Vulnerable Line
- Remediation Code
```
    temperature: 0.1
}

agent "security-validator" {
    model: "gpt-4o"
    instruction: "Review security findings. Reject any that are false positives or lack clear remediation."
}

# =============================================================================
# MDAP Pipeline: Vulnerability Scan
# =============================================================================

mdap_pipeline "global-security-audit" {
    strategy: ```
1. DECOMPOSE: Break the codebase into per-endpoint or per-module chunks.
2. ANALYZE: Use the auditor agent to find potential leaks.
3. VOTE: Use MDAP voting to ensure three separate samples agree on a vulnerability before flagging it (reduces noise).
4. RED-FLAG: Automatically reject any auditor finding that doesn't include valid remediation code.
```

    mdap_config {
        voting_strategy: "first-to-ahead-by-k"
        k: 4                      # Extremely high reliability for security
        parallel_samples: 5
        require_format: true
    }

    # Simulate a scan of 5000 source files
    total_steps: 5000

    microstep "audit-module" {
        use: agent("security-auditor")

        context: {
            module_name: $item.path
            source: $item.content
        }

        output_schema: {
            findings: [
                {
                    type: "string",
                    severity: "string",
                    line: "number",
                    remediation: "string"
                }
            ],
            risk_score: "number"
        }

        # Rejection sampling logic
        red_flags: {
            # Reject if remediation is missing or vague
            regex: ".*Consult documentation.*|.*Fix manually.*"
            # Reject if severity is not one of the allowed values
            not_in: { severity: ["Critical", "High", "Medium", "Low"] }
        }
    }
}

# =============================================================================
# Intent to Execute the Audit
# =============================================================================

intent "audit-codebase" {
    use: mdap_pipeline("global-security-audit")
    input: {
        target: "github.com/org/legacy-app"
        threshold: "High"
    }
}
