const error = category => Object.assign(new Error(category), { safeCategory: category });

export function chooseFormat(data, kind) {
  const formats = data.streamingData?.adaptiveFormats ?? [];
  const candidates = formats.filter(f => typeof f.mimeType === 'string' && f.mimeType.startsWith(kind + '/'));
  const clear = candidates.filter(f => !f.drmFamilies?.length && !f.drmTrackType && !f.fairPlayKeyUri && f.type !== 'FORMAT_STREAM_TYPE_OTF');
  const supported = clear.filter(f => {
    if (!(f.url || f.signatureCipher || f.cipher)) return false;
    if (kind === 'audio') return /codecs="(?:mp4a\.40\.2|opus)"/.test(f.mimeType);
    return /^video\/mp4;.*codecs="avc1\.[a-fA-F0-9]+"/.test(f.mimeType) &&
      Number.isSafeInteger(f.width) && Number.isSafeInteger(f.height) &&
      f.width > 0 && f.width <= 1920 && f.height > 0 && f.height <= 480;
  });
  if (!supported.length) {
    if (candidates.length && !clear.length) throw error('drm_unsupported');
    if (data.streamingData?.serverAbrStreamingUrl) throw error('sabr_unsupported');
    throw error(kind === 'audio' ? 'no_audio_format' : 'no_video_format');
  }
  supported.sort((a, b) => (kind === 'video' ? b.height - a.height : 0) ||
    (b.averageBitrate ?? b.bitrate ?? 0) - (a.averageBitrate ?? a.bitrate ?? 0));
  return { raw: supported[0], count: formats.length };
}

export function validateMediaURL(value) {
  if (typeof value !== 'string' || value.length > 65536 || /[\r\n\0'\\]/.test(value)) throw error('invalid_media_url');
  let target;
  try { target = new URL(value); } catch { throw error('invalid_media_url'); }
  if (target.protocol !== 'https:' || !target.hostname.endsWith('.googlevideo.com') || target.username || target.password || target.port || target.hash) throw error('invalid_media_url');
  if (target.searchParams.get('n')?.startsWith('enhanced_except_')) throw error('decipher_failed');
  if (target.searchParams.get('sabr') === '1') throw error('sabr_unsupported');
  return target;
}
