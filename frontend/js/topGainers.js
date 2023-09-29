const data = [
    { name: "Company 1", symbol: "ABC", mcap: "$1B", liq: "$100M", liq_mc: "10%", s30: "1.5", m1: "2.3", m5: "5.6", m10: "10.2", m30: "30.5" },
    { name: "Company 2", symbol: "XYZ", mcap: "$500M", liq: "$50M", liq_mc: "10%", s30: "1.2", m1: "1.8", m5: "4.2", m10: "8.1", m30: "28.3" },
    // Add more data rows as needed
];

// Get the table body element
const tbody = document.querySelector("tbody");

// Create table rows and populate data
data.forEach(item => {
    const row = document.createElement("tr");
    row.innerHTML = `
        <td>${item.name}</td>
        <td>${item.symbol}</td>
        <td>${item.mcap}</td>
        <td>${item.liq}</td>
        <td>${item.liq_mc}</td>
        <td>${item.s30}</td>
        <td>${item.m1}</td>
        <td>${item.m5}</td>
        <td>${item.m10}</td>
        <td>${item.m30}</td>
    `;
    tbody.appendChild(row);
});