const { Client } = require('pg');
const fs = require('fs');

async function extractSelfLid() {
    console.log("\n" + "═".repeat(60));
    console.log("🛡️ [SECURE LID SYSTEM] بوٹ کی اپنی آئی ڈی تلاش کی جا رہی ہے...");
    console.log("═".repeat(60));

    const client = new Client({
        connectionString: process.env.DATABASE_URL,
        ssl: { rejectUnauthorized: false }
    });

    try {
        await client.connect();
        console.log("✅ [DATABASE] پوسٹ گریس کے ساتھ لنک ہو گیا ہے۔");

        // 1. وہ نمبرز نکالیں جن سے بوٹ لاگ ان ہے
        const deviceRes = await client.query('SELECT jid FROM whatsmeow_device;');
        
        let botData = {};

        for (let row of deviceRes.rows) {
            const phoneJid = row.jid; // مثال: 92301...@s.whatsapp.net
            const pureNumber = phoneJid.split('@')[0].split(':')[0];

            console.log(`\n🔍 [CHECKING BOT] فون نمبر: ${pureNumber}`);

            // 2. اس نمبر کا پروفائل نام (Push Name) تلاش کریں
            const nameQuery = `SELECT push_name FROM whatsmeow_contacts WHERE jid = $1 LIMIT 1;`;
            const nameRes = await client.query(nameQuery, [phoneJid]);
            
            let botName = nameRes.rows[0]?.push_name;

            if (!botName) {
                console.log(`⚠️ [WARNING] نمبر ${pureNumber} کا ابھی کوئی نام نہیں ملا۔`);
                continue;
            }

            console.log(`👤 [PROFILE NAME] بوٹ کا نام ملا: "${botName}"`);

            // 3. اب اسی نام والی LID تلاش کریں (یہ وہی بوٹ ہوگا)
            const lidQuery = `
                SELECT jid FROM whatsmeow_contacts 
                WHERE push_name = $1 
                AND jid LIKE '%@lid' 
                LIMIT 1;
            `;
            const lidRes = await client.query(lidQuery, [botName]);

            if (lidRes.rows.length > 0) {
                const realLid = lidRes.rows[0].jid;
                console.log(`✅ [MATCH FOUND] آپ کی اصل LID مل گئی ہے: ${realLid}`);

                botData[pureNumber] = {
                    phone: pureNumber,
                    lid: realLid,
                    name: botName,
                    extractedAt: new Date().toISOString()
                };
            } else {
                console.log(`❌ [FAILED] اس نام کے ساتھ کوئی LID نہیں ملی۔ شاید ابھی سنک نہیں ہوئی۔`);
            }
        }

        // 4. فائنل ڈیٹا سیو کریں
        if (Object.keys(botData).length > 0) {
            fs.writeFileSync('./lid_data.json', JSON.stringify({ bots: botData }, null, 2));
            console.log("\n💾 [SUCCESS] بوٹ کا اپنا ڈیٹا 'lid_data.json' میں محفوظ ہے۔");
        }

    } catch (err) {
        console.error("❌ [ERROR]:", err.message);
    } finally {
        await client.end();
        console.log("🏁 [FINISHED]");
        console.log("═".repeat(60) + "\n");
        process.exit(0);
    }
}

extractSelfLid();