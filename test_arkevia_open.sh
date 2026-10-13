#!/bin/bash

echo "🧪 Testing Arkevia PDF Opening"
echo "================================"
echo

PDF_FILE=$(ls -t ~/.ssk/payslip/*.pdf 2>/dev/null | head -1)

if [ -z "$PDF_FILE" ]; then
    echo "❌ No PDF found in ~/.ssk/payslip/"
    echo "Run: ./bin/ssk arkevia download --latest"
    exit 1
fi

echo "📄 Found PDF: $(basename "$PDF_FILE")"
echo

if command -v xdg-open &> /dev/null; then
    echo "✅ xdg-open is available"
    echo "Testing file opening..."
    echo

    xdg-open "$PDF_FILE" &
    OPEN_PID=$!
    sleep 1

    if kill -0 $OPEN_PID 2>/dev/null; then
        echo "✅ File opened successfully (PID: $OPEN_PID)"
    else
        echo "⚠️  Command completed immediately (file may have opened)"
    fi
else
    echo "❌ xdg-open not found"
    echo "Install with: sudo apt-get install xdg-utils"
fi

echo
echo "================================"
echo "To test with ssk interactive mode:"
echo "  echo 'y' | ./bin/ssk arkevia download"