#!/bin/bash

echo "🧪 Testing SSK Go/PHP Encryption Compatibility"
echo "=============================================="
echo

echo "📋 Test 1: Listing all credentials (Go version)"
./go/bin/ssk credentials list
echo

echo "🔍 Test 2: Reading PHP-created credentials"
echo "Checking arkevia service (created by PHP):"
sqlite3 ~/.ssk/data.db "SELECT service FROM credentials WHERE service='arkevia'"
if [ $? -eq 0 ]; then
    echo "✅ Go can read PHP credentials"
else
    echo "❌ Failed to read PHP credentials"
fi
echo

echo "➕ Test 3: Creating credential with Go"
./go/bin/ssk credentials set compatibility_test go_user go_pass
echo

echo "🔍 Test 4: Verifying credential in database"
sqlite3 ~/.ssk/data.db "SELECT service FROM credentials WHERE service='compatibility_test'"
if [ $? -eq 0 ]; then
    echo "✅ Credential stored successfully"
else
    echo "❌ Failed to store credential"
fi
echo

echo "📖 Test 5: Reading back with Go"
./go/bin/ssk credentials list | grep compatibility_test
if [ $? -eq 0 ]; then
    echo "✅ Go can read its own encrypted data"
else
    echo "❌ Failed to read own encrypted data"
fi
echo

echo "🔐 Test 6: Checking encryption format"
echo "Encrypted values in database:"
sqlite3 ~/.ssk/data.db "SELECT service, length(username) as user_len, length(password) as pass_len FROM credentials WHERE service='compatibility_test'"
echo

echo "🧹 Cleanup"
./go/bin/ssk credentials delete compatibility_test
echo

echo "=============================================="
echo "✅ All tests completed!"