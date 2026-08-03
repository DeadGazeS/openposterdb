import { existsSync } from 'node:fs'
import { join, extname } from 'node:path'
import type { Plugin } from 'vite'
import { IMAGE_EXTS, IMAGE_DIRS, listImages } from './vite-plugin-image-utils'

const VIRTUAL_ID = 'virtual:placeholders'
const RESOLVED_ID = '\0' + VIRTUAL_ID

export default function placeholdersPlugin(): Plugin {
  let publicDir: string

  return {
    name: 'placeholders',

    configResolved(config) {
      publicDir = config.publicDir
    },

    resolveId(id) {
      if (id === VIRTUAL_ID) return RESOLVED_ID
    },

    async load(id) {
      if (id !== RESOLVED_ID) return

      let sharp: typeof import('sharp')
      try {
        sharp = (await import('sharp')).default
      } catch {
        // sharp is unavailable in some environments (e.g. the vitest module
        // runner). Fall back to an empty placeholder map so imports of
        // `virtual:placeholders` still resolve; BlurImage treats a missing
        // placeholder as "no placeholder".
        return 'export const placeholders = {};\n'
      }
      const entries: string[] = []

      for (const dir of IMAGE_DIRS) {
        const dirPath = join(publicDir, dir)
        if (!existsSync(dirPath)) continue
        const files = listImages(dirPath)

        for (const file of files) {
          const filePath = join(dirPath, file)
          const ext = extname(file).toLowerCase()
          const mime = ext === '.png' ? 'image/png' : 'image/jpeg'
          const format = ext === '.png' ? 'png' : 'jpeg'

          const buf = await sharp(filePath)
            .resize({ width: 20 })
            .toFormat(format, format === 'jpeg' ? { quality: 20 } : {})
            .toBuffer()

          const b64 = `data:${mime};base64,${buf.toString('base64')}`
          // Key by .webp extension to match the paths used in components
          const webpKey = file.replace(/\.(jpe?g|png)$/i, '.webp')
          entries.push(`  ${JSON.stringify('/' + dir + '/' + webpKey)}: ${JSON.stringify(b64)}`)
        }
      }

      return `export const placeholders = {\n${entries.join(',\n')}\n};\n`
    },

    configureServer(server) {
      for (const dir of IMAGE_DIRS) {
        const dirPath = join(publicDir, dir)
        server.watcher.add(dirPath)
        server.watcher.on('all', (event, path) => {
          if (path.startsWith(dirPath) && IMAGE_EXTS.has(extname(path).toLowerCase())) {
            const mod = server.moduleGraph.getModuleById(RESOLVED_ID)
            if (mod) {
              server.moduleGraph.invalidateModule(mod)
              server.hot.send({ type: 'full-reload' })
            }
          }
        })
      }
    },
  }
}
