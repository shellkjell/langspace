# Tower of Hanoi MDAP Pipeline - 5 Disks
# ========================================
#
# This example demonstrates MDAP (Massively Decomposed Agentic Processes)
# for solving Tower of Hanoi with 5 disks - requiring 2^5 - 1 = 31 steps.
#
# The MAKER framework ensures reliability through:
# - Maximal Decomposition: Each step handles exactly ONE disk move
# - First-to-ahead-by-k Voting: Multiple samples vote until k-margin consensus
# - Red-Flagging: Reject overly long or malformed responses
#
# This is a practical example that can be fully executed.

# =============================================================================
# Agent Definition
# =============================================================================

agent "hanoi-solver" {
    model: "gpt-4.1-mini"

    instruction: ```
You are a Tower of Hanoi solver. You follow a simple recursive strategy.

RULES:
1. Only one disk can be moved at a time
2. A larger disk cannot be placed on a smaller disk
3. All disks start on peg A, goal is to move all to peg C

STRATEGY (Recursive solution):
- To move n disks from source to target using auxiliary:
  1. Move n-1 disks from source to auxiliary (using target as temp)
  2. Move disk n from source to target
  3. Move n-1 disks from auxiliary to target (using source as temp)

For any given state, determine the next optimal move.
```

    temperature: 0.0
}

# =============================================================================
# Strategy File - Provided to every microstep
# =============================================================================

file "hanoi-strategy" {
    contents: ```
TOWER OF HANOI SOLUTION STRATEGY

Given the current state and previous move, determine the next move.

State format: Three pegs A, B, C, each containing a list of disk sizes (smallest to largest from top).
Example: A=[1,2,3], B=[], C=[] means peg A has disks 1,2,3 (1 on top).

For N disks, the optimal solution alternates moves based on parity:
- If N is odd: Cycle through A→C, A→B, B→C repeatedly
- If N is even: Cycle through A→B, A→C, B→C repeatedly

The smallest disk moves every other turn in a fixed cycle direction.
Non-smallest disk moves: there's only one legal move (the non-smallest moveable disk).

To determine next move:
1. If last move was smallest disk: find the only legal move for a non-smallest disk
2. If last move was not smallest (or first move): move smallest disk in cycle direction

Respond in exactly this format:
move = disk <N> from <source> to <target>
next_state = A=[...], B=[...], C=[...]
```
}

# =============================================================================
# MDAP Pipeline Configuration
# =============================================================================

mdap_pipeline "solve-hanoi-5" {
    # Reference to the strategy (provided to every microstep)
    strategy: file("hanoi-strategy")

    # MDAP Configuration
    mdap_config {
        # Voting: first sample to be k votes ahead wins
        voting_strategy: "first-to-ahead-by-k"
        k: 3

        # Parallel sampling
        parallel_samples: 3

        # Temperature settings (per MAKER paper)
        temperature_first: 0.0
        temperature_subsequent: 0.1

        # Red-flagging thresholds
        max_output_tokens: 500
        require_format: true

        # Checkpointing for recovery
        checkpoint_interval: 10
    }

    # Total steps for 5 disks: 2^5 - 1 = 31
    total_steps: 31

    # Initial state: all 5 disks on peg A
    # Disks numbered 1 (smallest) to 5 (largest)
    input: {
        num_disks: 5
        pegs: {
            A: [1, 2, 3, 4, 5]
            B: []
            C: []
        }
        target_peg: "C"
    }

    # Each step uses the same agent with minimal context
    microstep "move" {
        use: agent("hanoi-solver")

        # Minimal context - only what's needed for this one move
        context: {
            state: $current_state
            previous_move: $last_action
            strategy: file("hanoi-strategy")
        }

        # Expected output format
        output_schema: {
            move: "disk <N> from <source> to <target>"
            next_state: "A=[...], B=[...], C=[...]"
        }

        # Red flags that trigger rejection
        red_flags: {
            max_tokens: 500
            format_required: true
        }
    }

    # Success when all disks are on peg C
    success_condition: $state.pegs.C.length == $state.num_disks
}

# =============================================================================
# Intent to Run the Pipeline
# =============================================================================

intent "solve-hanoi" {
    use: mdap_pipeline("solve-hanoi-5")

    input: {
        num_disks: 5
        pegs: {
            A: [1, 2, 3, 4, 5]
            B: []
            C: []
        }
    }
}

# =============================================================================
# Smaller Test Configuration (3 disks = 7 steps)
# =============================================================================

mdap_pipeline "solve-hanoi-3" {
    strategy: file("hanoi-strategy")

    mdap_config {
        voting_strategy: "first-to-ahead-by-k"
        k: 3
        parallel_samples: 3
        temperature_first: 0.0
        temperature_subsequent: 0.1
        max_output_tokens: 300
        require_format: true
    }

    # 3 disks = 2^3 - 1 = 7 steps
    total_steps: 7

    input: {
        num_disks: 3
        pegs: {
            A: [1, 2, 3]
            B: []
            C: []
        }
        target_peg: "C"
    }

    microstep "move" {
        use: agent("hanoi-solver")

        context: {
            state: $current_state
            previous_move: $last_action
            strategy: file("hanoi-strategy")
        }

        output_schema: {
            move: "disk <N> from <source> to <target>"
            next_state: "A=[...], B=[...], C=[...]"
        }
    }
}

# Test intent for 3 disks
intent "test-hanoi" {
    use: mdap_pipeline("solve-hanoi-3")

    input: {
        num_disks: 3
        pegs: {
            A: [1, 2, 3]
            B: []
            C: []
        }
    }
}
