# Enterprise-Scale API Migration MDAP Pipeline
# ============================================
#
# This example demonstrates MDAP (Massively Decomposed Agentic Processes)
# for a massive software engineering task: migrating a legacy API
# across thousands of files with near-zero error tolerance.
#
# Task: Migrate `LegacyDate` to `ModernTime` across the codebase.
#
# MAKER Framework Benefits:
# - Maximal Decomposition: Each microstep handles exactly ONE file or ONE function
# - First-to-ahead-by-k Voting: Multiple samples vote on the diff to ensure correctness
# - Red-Flagging: Reject changes that break syntax (rejection sampling)
#
# New Feature: Runtime Inference with `auto`
# ==========================================
# The `auto` keyword enables dynamic inference of MDAP configuration parameters
# based on codebase analysis. This is useful when:
# - You don't know the task complexity upfront
# - You want the system to optimize parameters based on the codebase
# - You want bounds without specifying exact values
#
# Syntax:
#   k: auto                     # Fully automatic inference
#   k: auto(min: 2, max: 5)    # Constrained inference with bounds

# =============================================================================
# Agent Definition
# =============================================================================

agent "refactor-agent" {
    model: "claude-sonnet-4-20250514"

    instruction: ```
You are an expert software engineer specializing in large-scale refactoring.
Your task is to migrate code from the `LegacyDate` library to `ModernTime`.

Rules:
1. `LegacyDate.now()` -> `ModernTime.currentTimestamp()`
2. `LegacyDate.parse(s)` -> `ModernTime.fromISO(s)`
3. `LegacyDate.format(d, f)` -> `ModernTime.format(d, f)`
4. Ensure all imports are updated: remove `import { LegacyDate }` and add `import { ModernTime }`.

Only provide the diff of the changes. Do not explain your changes unless asked.
```

    temperature: 0.0
}

# =============================================================================
# Migration Strategy
# =============================================================================

file "migration-strategy" {
    contents: ```
MIGRATION STRATEGY: LEGACYDATE TO MODERNTIME

1. SCAN: Identify all usages of LegacyDate in the input code.
2. TRANSFORM: Apply the mapping rules provided in the agent instructions.
3. VALIDATE: Ensure the resulting code has no reference to LegacyDate.
4. FORMAT: Maintain the existing indentation and coding style.

Expected Output Format:
--- original
+++ modified
- <line removed>
+ <line added>
```
}

# =============================================================================
# MDAP Pipeline Configuration
# =============================================================================

mdap_pipeline "massive-api-migration" {
    strategy: file("migration-strategy")

    mdap_config {
        voting_strategy: "first-to-ahead-by-k"
        # Use auto-inference with bounds for k value
        # The runtime will analyze the codebase complexity and choose an optimal k
        # within the specified range. Higher k = more reliability, lower speed.
        k: auto(min: 2, max: 5)
        parallel_samples: 5       # Sample more aggressively for code tasks
        temperature_first: 0.0
        temperature_subsequent: 0.2
        max_output_tokens: 2000   # Large enough for medium file diffs
        require_format: true
    }

    # Use `infer` to dynamically determine total_steps from codebase
    # The runtime will use list_files() and count_matches() to estimate
    total_steps: infer

    # Input: List of files and their content (simplified for example)
    input: {
        repo: "enterprise-monorepo"
        batch_id: "2026-Q1-time-migration"
        target_files: [
            { path: "src/auth/session.ts", content: "..." },
            { path: "src/billing/invoice.ts", content: "..." },
            # ... 998 more files
        ]
    }

    # Microstep: Process exactly one file
    microstep "refactor-file" {
        use: agent("refactor-agent")

        context: {
            filename: $current_step_item.path
            source_code: $current_step_item.content
            strategy: file("migration-strategy")
        }

        # The MDAP runtime will repeat this microstep with multiple samples
        # until the voting consensus is reached for the diff.
        output_schema: {
            diff: "unified diff format"
            validation_status: "ok | error"
        }

        # Rejection sampling logic
        red_flags: {
            # Reject if the diff contains 'LegacyDate' (missed transformation)
            contains: "LegacyDate"
            # Reject if syntax looks broken (simple heuristic or tool call)
            regex: ".*<<<<<.*"
        }
    }

    # Success condition (optional, for monitoring)
    success_condition: $state.processed_count == $state.total_count
}

# =============================================================================
# Intent to Run the Refactor
# =============================================================================

intent "start-migration" {
    use: mdap_pipeline("massive-api-migration")

    input: {
        # Real-world usage would likely use a glob or file search tool
        target_files: [
            {
              path: "src/utils/logger.ts",
              content: "import { LegacyDate } from './lib';\nconst now = LegacyDate.now();"
            }
        ]
    }
}

# =============================================================================
# Another Example: Automated Test Suite Generation at Scale
# =============================================================================

agent "test-generator" {
    model: "gpt-4o"
    instruction: "Generate high-coverage Vitest unit tests for the provided function."
}

mdap_pipeline "massive-test-generation" {
    strategy: ```
For each function:
1. Identify all edge cases (null, undefined, large inputs).
2. Generate tests for the happy path.
3. Ensure 100% branch coverage.
```

    mdap_config {
        k: 2
        parallel_samples: 3
    }

    microstep "generate-test" {
        use: agent("test-generator")
        context: {
            func_name: $item.name
            func_code: $item.code
        }
        # Voting ensures the tests generated are consistent and valid
    }
}
