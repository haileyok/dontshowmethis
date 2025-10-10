import {LabelerServer} from '@skyware/labeler'
import Fastify from 'fastify'
import 'dotenv/config'

const LABELS: Record<string, boolean> = {
  'bad-faith': true,
  'off-topic': true,
  funny: true,
}

function run() {
  if (!process.env.SKYWARE_DID) {
    throw new Error('no did set in .env')
  }

  if (!process.env.SKYWARE_SIG_KEY) {
    throw new Error('no sig key set in .env')
  }

  if (!process.env.EMIT_LABEL_KEY) {
    throw new Error('no emit label key set in .env')
  }

  const labelerServer = new LabelerServer({
    did: process.env.SKYWARE_DID,
    signingKey: process.env.SKYWARE_SIG_KEY,
  })

  labelerServer.start(14831, error => {
    if (error) {
      console.error('Failed to start server:', error)
    } else {
      console.log('Labeler server running on port 14831')
    }
  })

  const fastify = Fastify({
    logger: true,
  })

  fastify.post('/emit', async (request, reply) => {
    if (
      !request.headers.authorization ||
      !request.headers.authorization.startsWith('Bearer') ||
      request.headers.authorization.split(' ')[1] !== process.env.EMIT_LABEL_KEY
    ) {
      reply.statusCode = 403
      return reply.send({error: 'unauthorized'})
    }

    const body = request.body as {uri?: string; label?: string}

    if (!body.uri) {
      reply.statusCode = 400
      return reply.send({error: 'must supply uri'})
    }

    if (!body.label) {
      reply.statusCode = 400
      return reply.send({error: 'must supply label'})
    }

    if (!LABELS[body.label]) {
      reply.statusCode = 400
      return reply.send({error: 'invalid label supplied'})
    }

    await labelerServer.createLabel({
      uri: body.uri,
      val: body.label,
    })

    reply.statusCode = 200
    return reply.send()
  })

  fastify.listen({port: 3000}, (err, _) => {
    if (err) {
      fastify.log.error(err)
      process.exit(1)
    }
  })
}

run()
