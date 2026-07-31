document.getElementById('connectBtn').addEventListener('click', async () => {
    const address = document.getElementById('address').value;
    const username = document.getElementById('username').value;
    const password = document.getElementById('password').value;
    const statusDiv = document.getElementById('status');

    statusDiv.textContent = 'Connecting...';
    statusDiv.style.color = '#fff';

    try {
        const response = await fetch('/api/connect', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ address, username, password })
        });

        if (response.ok) {
            statusDiv.textContent = 'Connection Successful! (Milestone 1 completed)';
            statusDiv.style.color = '#00ff00';
        } else {
            const errText = await response.text();
            statusDiv.textContent = 'Connection Failed: ' + errText;
            statusDiv.style.color = '#ff0000';
        }
    } catch (err) {
        statusDiv.textContent = 'Error: ' + err.message;
        statusDiv.style.color = '#ff0000';
    }
});
