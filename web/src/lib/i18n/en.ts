/*
 * English locale table (derived from messages/). Call sites that imported
 * `en` / MessageKey / writeErrorMessage from this path keep working.
 */
export { en, type MessageKey } from './catalog'
export { WRITE_ERROR_KEYS, writeErrorMessage } from './errors'
