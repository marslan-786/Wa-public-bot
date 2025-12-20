/**
 * 🔐 AUTOMATIC LID EXTRACTOR FOR RAILWAY DEPLOYMENT
 * Runs automatically when Go bot starts - NO MANUAL COMMANDS NEEDED
 * Extracts LID using Baileys and saves to MongoDB
 */

const { makeWASocket, useMultiFileAuthState, DisconnectReason } = require('@whiskeysockets/baileys');
const { Boom } = require('@hapi/boom');
const fs = require('fs');
const path = require('path');
const pino = require('pino');

// ════════════════════════════════════════════════════════════════
// 🔧 CONFIGURATION
// ════════════════════════════════════════════════════════════════

const CONFIG = {
    SESSIONS_DIR: './store',  // whatsmeow ka session folder
    OUTPUT_FILE: './lid_data.json',  // Temporary output for Go to read
    LOG_FILE: './lid_extractor.log',
    AUTO_EXIT_TIMEOUT: 30000, // 30 seconds
};

// ════════════════════════════════════════════════════════════════
// 📝 LOGGING SYSTEM
// ════════════════════════════════════════════════════════════════

const logger = {
    log: (msg) => {
        const timestamp = new Date().toISOString();
        const line = `[${timestamp}] ${msg}\n`;
        console.log(msg);
        fs.appendFileSync(CONFIG.LOG_FILE, line);
    },
    error: (msg) => {
        const timestamp = new Date().toISOString();
        const line = `[${timestamp}] ERROR: ${msg}\n`;
        console.error(`ERROR: ${msg}`);
        fs.appendFileSync(CONFIG.LOG_FILE, line);
    }
};

// ════════════════════════════════════════════════════════════════
// 🔍 LID EXTRACTION FUNCTIONS
// ════════════════════════════════════════════════════════════════

/**
 * Extract clean phone number from JID
 */
const getCleanNumber = (jid) => {
    if (!jid) return null;
    return jid.split('@')[0].split(':')[0].replace(/[^0-9]/g, '');
};

/**
 * Extract LID from Baileys credentials
 */
const extractLID = (creds) => {
    try {
        // Method 1: Direct LID field
        if (creds.me?.lid) {
            const cleanLID = getCleanNumber(creds.me.lid);
            logger.log(`✅ LID found (me.lid): ${cleanLID}`);
            return cleanLID;
        }

        // Method 2: User field (for linked devices)
        if (creds.me?.user) {
            const user = creds.me.user;
            const cleanUser = getCleanNumber(user);
            
            // LID is typically 13+ digits
            if (cleanUser && cleanUser.length > 13) {
                logger.log(`✅ LID found (me.user): ${cleanUser}`);
                return cleanUser;
            }
        }

        // Method 3: Parse from ID field
        if (creds.me?.id) {
            const id = creds.me.id;
            const parts = id.split(':');
            
            if (parts.length > 1) {
                const devicePart = parts[1].split('@')[0];
                if (devicePart && devicePart.length > 13) {
                    logger.log(`✅ LID found (me.id): ${devicePart}`);
                    return devicePart;
                }
            }
            
            // Try first part if second doesn't work
            const firstPart = getCleanNumber(parts[0]);
            if (firstPart && firstPart.length > 13) {
                logger.log(`✅ LID found (id.first): ${firstPart}`);
                return firstPart;
            }
        }

        logger.error('⚠️ LID not found in credentials');
        return null;
    } catch (err) {
        logger.error(`LID extraction error: ${err.message}`);
        return null;
    }
};

// ════════════════════════════════════════════════════════════════
// 📁 SESSION SCANNER
// ════════════════════════════════════════════════════════════════

/**
 * Scan single session directory for LID
 */
const scanSession = async (sessionPath, sessionId) => {
    try {
        logger.log(`\n━━━ Scanning: ${sessionId} ━━━`);
        
        // Check if creds.json exists
        const credsPath = path.join(sessionPath, 'creds.json');
        if (!fs.existsSync(credsPath)) {
            logger.error(`No creds.json found in ${sessionId}`);
            return null;
        }

        // Load credentials directly
        const credsData = fs.readFileSync(credsPath, 'utf-8');
        const creds = JSON.parse(credsData);
        
        if (!creds.me) {
            logger.error(`No 'me' field in credentials for ${sessionId}`);
            return null;
        }

        const phoneNumber = getCleanNumber(creds.me.id);
        const lid = extractLID(creds);
        const platform = creds.platform || 'Unknown';
        
        logger.log(`📞 Phone: ${phoneNumber || 'Unknown'}`);
        logger.log(`🆔 LID: ${lid || 'Not found'}`);
        logger.log(`📱 Platform: ${platform}`);
        
        if (phoneNumber && lid) {
            return {
                phone: phoneNumber,
                lid: lid,
                platform: platform,
                sessionId: sessionId,
                extractedAt: new Date().toISOString()
            };
        }

        return null;
    } catch (err) {
        logger.error(`Error scanning ${sessionId}: ${err.message}`);
        return null;
    }
};

/**
 * Scan all sessions in store directory
 */
const scanAllSessions = async () => {
    logger.log('\n╔═══════════════════════════════════════╗');
    logger.log('║   🔍 LID EXTRACTION STARTED          ║');
    logger.log('╚═══════════════════════════════════════╝\n');

    if (!fs.existsSync(CONFIG.SESSIONS_DIR)) {
        logger.error(`Sessions directory not found: ${CONFIG.SESSIONS_DIR}`);
        return [];
    }

    const results = [];
    
    // Check for whatsmeow session files
    const files = fs.readdirSync(CONFIG.SESSIONS_DIR);
    logger.log(`📁 Found ${files.length} file(s) in store directory`);

    // whatsmeow stores session in *.db files, but we need to check for auth files
    // Look for device-*.json or creds.json patterns
    for (const file of files) {
        const fullPath = path.join(CONFIG.SESSIONS_DIR, file);
        
        if (fs.statSync(fullPath).isDirectory()) {
            // Scan subdirectories
            const sessionData = await scanSession(fullPath, file);
            if (sessionData) {
                results.push(sessionData);
                logger.log(`✅ Extracted LID for: ${sessionData.phone}`);
            }
        } else if (file === 'creds.json') {
            // Single session in root
            const sessionData = await scanSession(CONFIG.SESSIONS_DIR, 'main');
            if (sessionData) {
                results.push(sessionData);
                logger.log(`✅ Extracted LID for: ${sessionData.phone}`);
            }
            break;
        }
    }

    // Try alternative approach: read database directly
    if (results.length === 0) {
        logger.log('\n⚠️ No sessions found via file scan, trying database approach...');
        
        // Check if impossible.db exists (SQLite database used by whatsmeow)
        const dbPath = './impossible.db';
        if (fs.existsSync(dbPath)) {
            logger.log('📊 Found impossible.db - attempting direct database read');
            // Note: This requires sqlite3 package, which we'll handle in Go
            logger.log('⚠️ Database direct read requires Go integration');
        }
    }

    logger.log(`\n📊 Total LIDs extracted: ${results.length}`);
    return results;
};

// ════════════════════════════════════════════════════════════════
// 💾 OUTPUT HANDLING
// ════════════════════════════════════════════════════════════════

/**
 * Save extracted data to file for Go to read
 */
const saveResults = (results) => {
    try {
        const output = {
            timestamp: new Date().toISOString(),
            count: results.length,
            bots: {}
        };

        results.forEach(bot => {
            output.bots[bot.phone] = {
                phone: bot.phone,
                lid: bot.lid,
                platform: bot.platform,
                sessionId: bot.sessionId,
                extractedAt: bot.extractedAt
            };
        });

        fs.writeFileSync(CONFIG.OUTPUT_FILE, JSON.stringify(output, null, 2));
        logger.log(`\n✅ Results saved to: ${CONFIG.OUTPUT_FILE}`);
        
        return true;
    } catch (err) {
        logger.error(`Failed to save results: ${err.message}`);
        return false;
    }
};

// ════════════════════════════════════════════════════════════════
// 🚀 MAIN EXECUTION
// ════════════════════════════════════════════════════════════════

const main = async () => {
    const startTime = Date.now();
    
    try {
        // Clear old log
        if (fs.existsSync(CONFIG.LOG_FILE)) {
            fs.unlinkSync(CONFIG.LOG_FILE);
        }

        logger.log('🚀 LID Extractor Starting...');
        logger.log(`📂 Sessions directory: ${CONFIG.SESSIONS_DIR}`);
        logger.log(`📄 Output file: ${CONFIG.OUTPUT_FILE}\n`);

        // Scan all sessions
        const results = await scanAllSessions();

        // Save results
        const saved = saveResults(results);

        const duration = ((Date.now() - startTime) / 1000).toFixed(2);
        logger.log(`\n⏱️ Extraction completed in ${duration}s`);

        // Exit with appropriate code
        if (results.length > 0 && saved) {
            logger.log('✅ SUCCESS: LIDs extracted and saved');
            process.exit(0);
        } else {
            logger.log('⚠️ WARNING: No LIDs found (might be normal on first run)');
            process.exit(1);
        }

    } catch (err) {
        logger.error(`Fatal error: ${err.message}`);
        logger.error(err.stack);
        process.exit(1);
    }
};

// ════════════════════════════════════════════════════════════════
// 🎯 ENTRY POINT
// ════════════════════════════════════════════════════════════════

// Auto-run when executed directly
if (require.main === module) {
    main().catch(err => {
        logger.error(`Unhandled error: ${err.message}`);
        process.exit(1);
    });

    // Safety timeout
    setTimeout(() => {
        logger.error('⏱️ Timeout reached - force exit');
        process.exit(1);
    }, CONFIG.AUTO_EXIT_TIMEOUT);
}

// Export for programmatic use
module.exports = {
    scanAllSessions,
    extractLID,
    saveResults
};