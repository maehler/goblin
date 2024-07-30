webSocket = new WebSocket("/ws");
webSocket.onmessage = function (event) {
    let d = JSON.parse(event.data);
    if (d.systemType == "device") {
        console.log(`setting ${d.capability} for ${d.sourceNode}`);
        let el = document.querySelector(`div[data-device-id='${d.sourceNode}'] .${d.capability}-value`);
        if (el) {
            el.textContent = d.value;
        }
    }
    console.log(d);
}
