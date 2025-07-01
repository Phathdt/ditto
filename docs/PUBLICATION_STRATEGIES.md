# Publication Strategies for Ditto WAL Listener

## Overview

Ditto supports two strategies for managing PostgreSQL publications:

1. **Single Publication** - All tables in one publication (recommended)
2. **Multiple Publications** - Each table gets its own publication

## 1. Single Publication Strategy (Default)

### When to use:
- ✅ Simple architecture with one listener
- ✅ All tables have similar processing requirements
- ✅ Resource efficiency is important
- ✅ Easy maintenance and monitoring

### Configuration:
```yaml
publication_strategy: "single"  # or omit (default)
prefix_watch_list: "events"

watch_list:
  deposit_events:
    mapping: "deposits"
  withdraw_events:
    mapping: "withdrawals"
  loan_events:
    mapping: "loans"
```

### Results in:
- **Publication**: `ditto`
- **Tables**: `deposit_events, withdraw_events, loan_events`
- **Topics**: `events.deposits`, `events.withdrawals`, `events.loans`

## 2. Multiple Publications Strategy

### When to use:
- ✅ Need different consumers for different tables
- ✅ Tables have different processing requirements
- ✅ Want fault isolation between tables
- ✅ Planning to scale with microservices

### Configuration:
```yaml
publication_strategy: "multiple"
publication_prefix: "ditto"
prefix_watch_list: "events"

watch_list:
  deposit_events:
    mapping: "deposits"
  withdraw_events:
    mapping: "withdrawals"
  loan_events:
    mapping: "loans"
```

### Results in:
- **Publications**: `ditto_deposit_events`, `ditto_withdraw_events`, `ditto_loan_events`
- **Tables**: Each publication contains one table
- **Topics**: `events.deposits`, `events.withdrawals`, `events.loans`

## Comparison Table

| Aspect | Single Publication | Multiple Publications |
|--------|-------------------|---------------------|
| **Simplicity** | ✅ Simple | ❌ Complex |
| **Resource Usage** | ✅ Low | ❌ Higher |
| **Fault Isolation** | ❌ Coupled | ✅ Isolated |
| **Scalability** | ❌ Limited | ✅ High |
| **Maintenance** | ✅ Easy | ❌ More work |
| **Replication Slots** | 1 slot | N slots |

## Migration Between Strategies

### From Single to Multiple:
1. Update config with `publication_strategy: "multiple"`
2. Restart the service
3. Old `ditto` publication will be dropped
4. New `ditto_*` publications will be created

### From Multiple to Single:
1. Update config with `publication_strategy: "single"` (or remove)
2. Restart the service
3. All `ditto_*` publications will be dropped
4. New single `ditto` publication will be created

## Manual Verification

Use the provided SQL script to check publications:

```bash
psql -d your_database -f scripts/check_publications.sql
```

## Recommendation

**Start with Single Publication** unless you have specific requirements for Multiple Publications. You can always migrate later when your system grows.
