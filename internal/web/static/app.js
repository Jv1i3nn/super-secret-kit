function runCommand(command, args) {
    const consoleDiv = document.getElementById('console');

    // Add command to console
    const cmdLine = document.createElement('div');
    cmdLine.className = 'console-line system';
    cmdLine.textContent = `> ssk ${command} ${args.join(' ')}`;
    consoleDiv.appendChild(cmdLine);
    consoleDiv.scrollTop = consoleDiv.scrollHeight;

    fetch('/api/run', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            command: command,
            args: args
        })
    })
        .then(response => response.json())
        .then(data => {
            const outputLine = document.createElement('div');
            outputLine.className = data.success ? 'console-line success' : 'console-line error';
            outputLine.textContent = data.output;
            consoleDiv.appendChild(outputLine);
            consoleDiv.scrollTop = consoleDiv.scrollHeight;
        })
        .catch(error => {
            const errorLine = document.createElement('div');
            errorLine.className = 'console-line error';
            errorLine.textContent = `Error: ${error.message}`;
            consoleDiv.appendChild(errorLine);
            consoleDiv.scrollTop = consoleDiv.scrollHeight;
        });
}

function clearConsole() {
    const consoleDiv = document.getElementById('console');
    consoleDiv.innerHTML = '<div class="console-line system">> Console cleared.</div>';
}
