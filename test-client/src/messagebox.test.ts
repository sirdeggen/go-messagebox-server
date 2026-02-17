/**
 * Integration tests for the Go MessageBox Server
 * Uses @bsv/message-box-client (MessageBoxClient) with @bsv/sdk (ProtoWallet)
 * pointing at the local Go server on localhost:8080.
 *
 * Based on: https://fast.brc.dev/?snippet=messageDelivery
 * Reference: https://github.com/bsv-blockchain/message-box-client/blob/master/src/__tests/integration/integrationHTTP.test.ts
 *
 * Prerequisites:
 *   1. Start the Go server: cd ../; go run ./cmd/server
 *   2. Run tests: npx jest
 */

import { PrivateKey, ProtoWallet } from '@bsv/sdk'
import { MessageBoxClient } from '@bsv/message-box-client'

const SERVER_HOST = process.env.MESSAGEBOX_HOST || 'http://localhost:8080'

// Create two ProtoWallet instances (sender + recipient) for testing
// ProtoWallet is a lightweight in-memory wallet that implements WalletInterface
const senderKey = PrivateKey.fromRandom()
const recipientKey = PrivateKey.fromRandom()
const senderWallet = new ProtoWallet(senderKey)
const recipientWallet = new ProtoWallet(recipientKey)

const senderIdentityKey = senderKey.toPublicKey().toString()
const recipientIdentityKey = recipientKey.toPublicKey().toString()

// Create MessageBoxClients for sender and recipient
const senderClient = new MessageBoxClient({
  host: SERVER_HOST,
  walletClient: senderWallet as any,
  enableLogging: false,
  networkPreset: 'local'
})

const recipientClient = new MessageBoxClient({
  host: SERVER_HOST,
  walletClient: recipientWallet as any,
  enableLogging: false,
  networkPreset: 'local'
})

describe('Go MessageBox Server — HTTP Integration Tests', () => {
  let testMessageId: string

  // ─── Test 1: Send a plaintext message ───────────────────────────────
  test('should send a message successfully (skipEncryption)', async () => {
    const response = await senderClient.sendMessage({
      recipient: recipientIdentityKey,
      messageBox: 'test_inbox',
      body: 'Hello from the integration test!',
      skipEncryption: true
    })

    expect(response).toHaveProperty('status', 'success')
    expect(response).toHaveProperty('messageId')
    expect(typeof response.messageId).toBe('string')
    expect(response.messageId.length).toBeGreaterThan(0)

    testMessageId = response.messageId
  })

  // ─── Test 2: Recipient can list messages ────────────────────────────
  test('should list messages in the recipient messageBox', async () => {
    const messages = await recipientClient.listMessages({
      messageBox: 'test_inbox'
    })

    expect(Array.isArray(messages)).toBe(true)
    expect(messages.length).toBeGreaterThan(0)

    const found = messages.find(
      (m: any) => m.body === 'Hello from the integration test!'
    )
    expect(found).toBeDefined()
    expect(found!.sender).toBe(senderIdentityKey)
  })

  // ─── Test 3: Listing an empty box returns [] ────────────────────────
  test('should return empty array for a box with no messages', async () => {
    const messages = await recipientClient.listMessages({
      messageBox: 'nonexistent_box'
    })

    expect(messages).toEqual([])
  })

  // ─── Test 4: Acknowledge (delete) a message ─────────────────────────
  test('should acknowledge a message', async () => {
    const result = await recipientClient.acknowledgeMessage({
      messageIds: [testMessageId]
    })

    expect(result).toBe('success')
  })

  // ─── Test 5: Message is gone after acknowledgment ───────────────────
  test('should not list acknowledged message', async () => {
    const messages = await recipientClient.listMessages({
      messageBox: 'test_inbox'
    })

    const found = messages.find((m: any) => m.messageId === testMessageId)
    expect(found).toBeUndefined()
  })

  // ─── Test 6: Fail to acknowledge nonexistent message ────────────────
  test('should fail to acknowledge a nonexistent message', async () => {
    await expect(
      recipientClient.acknowledgeMessage({
        messageIds: ['totally-fake-message-id']
      })
    ).rejects.toThrow()
  })

  // ─── Test 7: Reject send with empty recipient ──────────────────────
  test('should fail if recipient is empty', async () => {
    await expect(
      senderClient.sendMessage({
        recipient: '',
        messageBox: 'test_inbox',
        body: 'This should fail'
      })
    ).rejects.toThrow()
  })

  // ─── Test 8: Reject send with empty body ───────────────────────────
  test('should fail if message body is empty', async () => {
    await expect(
      senderClient.sendMessage({
        recipient: recipientIdentityKey,
        messageBox: 'test_inbox',
        body: ''
      })
    ).rejects.toThrow()
  })

  // ─── Test 9: Send multiple messages, list all ──────────────────────
  test('should handle multiple messages in the same box', async () => {
    const msg1 = await senderClient.sendMessage({
      recipient: recipientIdentityKey,
      messageBox: 'multi_box',
      body: 'Message one',
      skipEncryption: true
    })
    const msg2 = await senderClient.sendMessage({
      recipient: recipientIdentityKey,
      messageBox: 'multi_box',
      body: 'Message two',
      skipEncryption: true
    })

    expect(msg1.status).toBe('success')
    expect(msg2.status).toBe('success')

    const messages = await recipientClient.listMessages({
      messageBox: 'multi_box'
    })

    expect(messages.length).toBeGreaterThanOrEqual(2)
    const bodies = messages.map((m: any) => m.body)
    expect(bodies).toContain('Message one')
    expect(bodies).toContain('Message two')

    // Clean up
    await recipientClient.acknowledgeMessage({
      messageIds: [msg1.messageId, msg2.messageId]
    })
  })

  // ─── Test 10: Send JSON body ───────────────────────────────────────
  test('should send and receive a JSON object body', async () => {
    const jsonBody = { type: 'payment', amount: 1000, memo: 'test' }
    const response = await senderClient.sendMessage({
      recipient: recipientIdentityKey,
      messageBox: 'json_box',
      body: JSON.stringify(jsonBody),
      skipEncryption: true
    })

    expect(response.status).toBe('success')

    const messages = await recipientClient.listMessages({
      messageBox: 'json_box'
    })

    const found = messages.find(
      (m: any) => m.messageId === response.messageId
    )
    expect(found).toBeDefined()

    // Body comes back as string — parse it if it's valid JSON, otherwise check as-is
    const body = found!.body
    const parsed = typeof body === 'string' ? JSON.parse(body) : body
    expect(parsed.type).toBe('payment')
    expect(parsed.amount).toBe(1000)

    // Clean up
    await recipientClient.acknowledgeMessage({
      messageIds: [response.messageId]
    })
  })

  // ─── Test 11: Sender sends to self ─────────────────────────────────
  test('should allow sending a message to self', async () => {
    const response = await senderClient.sendMessage({
      recipient: senderIdentityKey,
      messageBox: 'self_box',
      body: 'Note to self',
      skipEncryption: true
    })

    expect(response.status).toBe('success')

    const messages = await senderClient.listMessages({
      messageBox: 'self_box'
    })

    expect(messages.length).toBeGreaterThan(0)
    expect(messages.some((m: any) => m.body === 'Note to self')).toBe(true)

    // Clean up
    await senderClient.acknowledgeMessage({
      messageIds: [response.messageId]
    })
  })
})
