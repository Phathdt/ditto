-- Script to check and manage PostgreSQL publications for Ditto
-- Usage: psql -d your_database -f scripts/check_publications.sql

-- Set variables (modify these as needed)
\set expected_tables '''deposit_events'', ''withdraw_events'', ''loan_events'''
\set publication_name '''ditto'''

-- 1. Check current publications
SELECT
    'CURRENT PUBLICATIONS' as info,
    pubname,
    puballtables,
    pubinsert,
    pubupdate,
    pubdelete
FROM pg_publication
ORDER BY pubname;

-- 2. Check tables in specific publication
SELECT
    'TABLES IN PUBLICATION' as info,
    p.pubname,
    t.schemaname,
    t.tablename
FROM pg_publication p
LEFT JOIN pg_publication_tables t ON p.pubname = t.pubname
WHERE p.pubname = :publication_name
ORDER BY t.tablename;

-- 3. Check if expected tables exist
WITH expected_tables_cte AS (
    SELECT unnest(ARRAY[:expected_tables]) as table_name
),
table_status AS (
    SELECT
        e.table_name,
        CASE
            WHEN t.table_name IS NOT NULL THEN 'EXISTS'
            ELSE 'MISSING'
        END as status,
        t.table_type
    FROM expected_tables_cte e
    LEFT JOIN information_schema.tables t
        ON t.table_name = e.table_name
        AND t.table_schema = 'public'
)
SELECT
    'TABLE STATUS' as info,
    table_name,
    status,
    table_type
FROM table_status
ORDER BY table_name;

-- 4. Compare expected vs current publication tables
WITH expected_tables_cte AS (
    SELECT unnest(ARRAY[:expected_tables]) as table_name
),
current_tables AS (
    SELECT t.tablename as table_name
    FROM pg_publication p
    JOIN pg_publication_tables t ON p.pubname = t.pubname
    WHERE p.pubname = :publication_name
),
comparison AS (
    SELECT
        COALESCE(e.table_name, c.table_name) as table_name,
        CASE
            WHEN e.table_name IS NOT NULL AND c.table_name IS NOT NULL THEN 'MATCH'
            WHEN e.table_name IS NOT NULL AND c.table_name IS NULL THEN 'MISSING_IN_PUB'
            WHEN e.table_name IS NULL AND c.table_name IS NOT NULL THEN 'EXTRA_IN_PUB'
        END as status
    FROM expected_tables_cte e
    FULL OUTER JOIN current_tables c ON e.table_name = c.table_name
)
SELECT
    'PUBLICATION COMPARISON' as info,
    table_name,
    status
FROM comparison
ORDER BY
    CASE status
        WHEN 'MATCH' THEN 1
        WHEN 'MISSING_IN_PUB' THEN 2
        WHEN 'EXTRA_IN_PUB' THEN 3
    END,
    table_name;

-- 5. Generate sync commands (if needed)
WITH expected_tables_cte AS (
    SELECT unnest(ARRAY[:expected_tables]) as table_name
),
current_tables AS (
    SELECT t.tablename as table_name
    FROM pg_publication p
    JOIN pg_publication_tables t ON p.pubname = t.pubname
    WHERE p.pubname = :publication_name
),
expected_set AS (
    SELECT string_agg(table_name, ',' ORDER BY table_name) as expected_list
    FROM expected_tables_cte
),
current_set AS (
    SELECT string_agg(table_name, ',' ORDER BY table_name) as current_list
    FROM current_tables
),
sync_needed AS (
    SELECT
        CASE
            WHEN COALESCE(e.expected_list, '') != COALESCE(c.current_list, '')
            THEN true
            ELSE false
        END as needs_sync,
        e.expected_list,
        c.current_list
    FROM expected_set e
    CROSS JOIN current_set c
)
SELECT
    'SYNC STATUS' as info,
    CASE
        WHEN needs_sync THEN 'PUBLICATION NEEDS UPDATE'
        ELSE 'PUBLICATION IS UP TO DATE'
    END as status,
    'Expected: ' || COALESCE(expected_list, 'none') as expected,
    'Current: ' || COALESCE(current_list, 'none') as current
FROM sync_needed;

-- 6. SQL commands to fix publication (uncomment to execute)
/*
-- Drop and recreate publication with correct tables
DROP PUBLICATION IF EXISTS ditto;
CREATE PUBLICATION ditto FOR TABLE deposit_events, withdraw_events, loan_events;

-- Or for ALL TABLES:
-- CREATE PUBLICATION ditto FOR ALL TABLES;

-- Verify the fix
SELECT
    p.pubname,
    array_agg(t.tablename ORDER BY t.tablename) as tables
FROM pg_publication p
LEFT JOIN pg_publication_tables t ON p.pubname = t.pubname
WHERE p.pubname = 'ditto'
GROUP BY p.pubname;
*/
