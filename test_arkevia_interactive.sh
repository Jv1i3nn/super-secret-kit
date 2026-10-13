#!/bin/bash

echo "🧪 Testing Arkevia Interactive Mode with PDF Opening"
echo "====================================================="
echo
echo "This test will:"
echo "1. List available documents"
echo "2. Select document #1"
echo "3. Ask to open the file (answer 'n' to skip opening)"
echo
echo "Press Enter to continue..."
read

echo -e "1\nn" | ./bin/ssk arkevia download

echo
echo "====================================================="
echo "✅ Test completed"
echo
echo "To test PDF opening manually:"
echo "  1. Run: ./bin/ssk arkevia download"
echo "  2. Select a document number"
echo "  3. Answer 'y' when asked to open the file"