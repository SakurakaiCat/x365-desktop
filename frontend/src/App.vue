<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import {
  GetNodes, AddNode, ImportFromText, RemoveNode, RenameNode,
  Connect, Disconnect, GetStatus, GetSettings, SaveSettings, SetSystemProxy,
  GetExitInfo, TestLatency, GetLogs, ClearLogs, GetTraffic
} from '../wailsjs/go/main/App'
import { EventsOn, EventsOff, WindowSetSystemDefaultTheme } from '../wailsjs/runtime/runtime'
import * as models from '../wailsjs/go/models'

type Node = models.nodestore.Node
type Settings = models.nodestore.Settings
type StatusDTO = models.main.StatusDTO
type ExitInfoDTO = models.main.ExitInfoDTO
type TrafficDTO = models.main.TrafficDTO

// ── OS theme tracking ──
const isDark = ref(true)
let themeMedia: MediaQueryList | null = null

function updateTheme() {
  isDark.value = !themeMedia || themeMedia.matches
  document.documentElement.setAttribute('data-theme', isDark.value ? 'dark' : 'light')
}

const activeTab = ref<'nodes' | 'settings' | 'about'>('nodes')
const nodes = ref<Node[]>([])
const status = ref<StatusDTO | null>(null)
const settings = ref<Settings | null>(null)
const exitInfo = ref<ExitInfoDTO | null>(null)
const traffic = ref<TrafficDTO | null>(null)
const logs = ref<string[]>([])
const importText = ref('')
const importExpanded = ref(false)
const errorMsg = ref('')
const testingIds = ref<Set<string>>(new Set())
const latencies = ref<Record<string, number>>({})
const showAddr = ref<Record<string, boolean>>({})
const editTarget = ref<Node | null>(null)
const editName = ref('')
const editShowSensitive = ref(false)
const logContainer = ref<HTMLElement | null>(null)

// ── Helpers ──

const isoToFlag = (iso?: string): string => {
  if (!iso || iso.length !== 2) return '🌐'
  const base = 0x1f1e6 - 'A'.charCodeAt(0)
  return String.fromCodePoint(base + iso.charCodeAt(0), base + iso.charCodeAt(1))
}

const formatBytes = (bytes: number): string => {
  if (bytes >= 1 << 30) return (bytes / (1 << 30)).toFixed(1) + ' GB'
  if (bytes >= 1 << 20) return (bytes / (1 << 20)).toFixed(1) + ' MB'
  if (bytes >= 1 << 10) return (bytes / (1 << 10)).toFixed(1) + ' KB'
  return bytes + ' B'
}

const maskOr = (value: string | undefined, visible: boolean): string => {
  if (!value) return '—'
  if (visible) return value
  if (value.length <= 8) return '••••••••'
  return value.slice(0, 4) + '••••' + value.slice(-4)
}

const latencyClass = (ms: number): string => {
  if (ms < 300) return 'latency-good'
  if (ms < 800) return 'latency-mid'
  return 'latency-bad'
}

const connectedNode = computed(() => {
  if (!status.value?.connectedId) return null
  return nodes.value.find(n => n.id === status.value?.connectedId) || null
})

// ── Data refresh ──

async function refresh() {
  try {
    const [n, s, st] = await Promise.all([GetNodes(), GetSettings(), GetStatus()])
    nodes.value = n || []
    settings.value = s
    status.value = st
  } catch (e: any) {
    errorMsg.value = String(e)
  }
}

async function refreshLogs() {
  try {
    logs.value = await GetLogs()
  } catch {}
}

async function refreshExitInfo() {
  if (!status.value?.running) {
    exitInfo.value = null
    return
  }
  try {
    const info = await GetExitInfo()
    exitInfo.value = info
  } catch (e: any) {
    // silent
  }
}

// ── Actions ──

async function doConnect(nodeID: string) {
  errorMsg.value = ''
  try {
    await Connect(nodeID)
    await refresh()
    await refreshExitInfo()
  } catch (e: any) {
    errorMsg.value = String(e)
  }
}

async function doDisconnect() {
  errorMsg.value = ''
  try {
    await Disconnect()
    await refresh()
    exitInfo.value = null
    traffic.value = null
  } catch (e: any) {
    errorMsg.value = String(e)
  }
}

async function importFromText() {
  if (!importText.value.trim()) return
  try {
    const count = await ImportFromText(importText.value)
    importText.value = ''
    importExpanded.value = false
    await refresh()
  } catch (e: any) {
    errorMsg.value = String(e)
  }
}

async function removeNode(id: string) {
  try {
    await RemoveNode(id)
    await refresh()
  } catch (e: any) {
    errorMsg.value = String(e)
  }
}

async function doTestLatency(node: Node) {
  if (testingIds.value.has(node.id)) return
  testingIds.value = new Set([...testingIds.value, node.id])
  try {
    const ms = await TestLatency(node.id)
    latencies.value = { ...latencies.value, [node.id]: ms }
  } catch {
    const copy = { ...latencies.value }
    delete copy[node.id]
    latencies.value = copy
  } finally {
    const copy = new Set(testingIds.value)
    copy.delete(node.id)
    testingIds.value = copy
  }
}

function toggleAddr(id: string) {
  showAddr.value = { ...showAddr.value, [id]: !showAddr.value[id] }
}

function openEdit(node: Node) {
  editTarget.value = node
  editName.value = node.label
  editShowSensitive.value = false
}

async function saveEdit() {
  if (!editTarget.value) return
  try {
    await RenameNode(editTarget.value.id, editName.value)
    editTarget.value = null
    await refresh()
  } catch (e: any) {
    errorMsg.value = String(e)
  }
}

async function deleteFromEdit() {
  if (!editTarget.value) return
  await removeNode(editTarget.value.id)
  editTarget.value = null
}

async function toggleSysProxy() {
  if (!status.value) return
  try {
    await SetSystemProxy(!status.value.sysProxyOn)
    await refresh()
  } catch (e: any) {
    errorMsg.value = String(e)
  }
}

async function saveSettings() {
  if (!settings.value) return
  try {
    await SaveSettings(settings.value)
    await refresh()
  } catch (e: any) {
    errorMsg.value = String(e)
  }
}

async function doClearLogs() {
  await ClearLogs()
  await refreshLogs()
}

// ── Lifecycle ──

onMounted(async () => {
  // Follow Windows theme: set native window title bar + CSS variables
  try { WindowSetSystemDefaultTheme() } catch {}
  themeMedia = window.matchMedia('(prefers-color-scheme: dark)')
  updateTheme()
  themeMedia.addEventListener('change', updateTheme)

  await refresh()
  await refreshLogs()
  EventsOn('proxy:connected', async () => { await refresh(); await refreshExitInfo() })
  EventsOn('proxy:disconnected', async () => { await refresh(); exitInfo.value = null; traffic.value = null })
  EventsOn('proxy:traffic', (t: TrafficDTO) => { traffic.value = t })
  EventsOn('proxy:log', async () => { await refreshLogs() })
})

onUnmounted(() => {
  if (themeMedia) themeMedia.removeEventListener('change', updateTheme)
  EventsOff('proxy:connected')
  EventsOff('proxy:disconnected')
  EventsOff('proxy:traffic')
  EventsOff('proxy:log')
})

// Auto-scroll logs to bottom
watch(logs, async () => {
  await nextTick()
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
})
</script>

<template>
  <div class="app">
    <!-- Top bar -->
    <div class="topbar">
      <div class="topbar-title">365 Open365VPN</div>
      <div class="tabs">
        <div class="tab" :class="{ active: activeTab === 'nodes' }" @click="activeTab = 'nodes'">节点</div>
        <div class="tab" :class="{ active: activeTab === 'settings' }" @click="activeTab = 'settings'">设置</div>
        <div class="tab" :class="{ active: activeTab === 'about' }" @click="activeTab = 'about'">关于</div>
      </div>
    </div>

    <!-- Nodes tab -->
    <template v-if="activeTab === 'nodes'">
      <div class="content-scroll">
        <!-- Status dashboard -->
        <div class="status-card" :class="{ connected: status?.running, connecting: status?.running && !status?.tunRunning && status?.tunMode }">
          <div class="status-header">
            <span v-if="status?.running" class="flag">{{ isoToFlag(connectedNode?.countryCode) }}</span>
            <span class="status-label">{{ status?.running ? (status.currentLabel || 'Open365VPN') : 'Open365VPN' }}</span>
          </div>
          <div class="status-row">
            <span class="status-dot" :class="{ on: status?.running }"></span>
            <span v-if="status?.running" class="status-text connected-text">已连接
              <span v-if="status.tunRunning" class="badge-tun">TUN</span>
              <span v-else-if="status.sysProxyOn" class="badge-proxy">系统代理</span>
            </span>
            <span v-else class="status-text">未连接</span>
          </div>

          <!-- Traffic -->
          <div v-if="traffic && status?.running" class="traffic-row">
            ↓ {{ formatBytes(traffic.download) }} &nbsp; ↑ {{ formatBytes(traffic.upload) }}
          </div>

          <!-- Exit info -->
          <div v-if="exitInfo && status?.running" class="exit-info">
            <div class="exit-row"><span class="exit-key">出口 IP</span><span class="exit-val">{{ exitInfo.ip }}</span></div>
            <div class="exit-row"><span class="exit-key">归属地</span><span class="exit-val">{{ exitInfo.country }}<span v-if="exitInfo.countryCode"> ({{ exitInfo.countryCode }})</span></span></div>
            <div class="exit-row"><span class="exit-key">ASN</span><span class="exit-val">{{ exitInfo.asn }}</span></div>
            <div v-if="exitInfo.org && exitInfo.org !== '—'" class="exit-row"><span class="exit-key">运营商</span><span class="exit-val">{{ exitInfo.org }}</span></div>
            <button class="btn-sm" @click="refreshExitInfo">刷新</button>
          </div>

          <!-- Disconnect button -->
          <button v-if="status?.running" class="btn btn-danger disconnect-btn" @click="doDisconnect">断开连接</button>
        </div>

        <!-- Node section header -->
        <div class="section-header">
          <span class="section-title">节点</span>
          <span class="section-count">{{ nodes.length }} 个</span>
          <button class="icon-btn" @click="importExpanded = !importExpanded">{{ importExpanded ? '收起' : '添加' }}</button>
        </div>

        <!-- Import area -->
        <div v-if="importExpanded" class="import-card">
          <textarea v-model="importText" placeholder="粘贴 x365:// URI，每行一个" rows="4"></textarea>
          <button class="btn btn-primary" @click="importFromText">导入</button>
        </div>

        <!-- Node list -->
        <div v-if="nodes.length === 0" class="empty-state">
          <p>暂无节点，点击「添加」导入 x365:// URI</p>
        </div>
        <div
          v-for="node in nodes"
          :key="node.id"
          class="node-item"
          :class="{ active: status?.connectedId === node.id }"
          @click="doConnect(node.id)"
        >
          <div class="node-flag">{{ isoToFlag(node.countryCode) }}</div>
          <div class="node-info">
            <div class="node-label-row">
              <span class="node-label">{{ node.label }}</span>
              <span v-if="testingIds.has(node.id)" class="testing-spinner"></span>
              <span v-else-if="latencies[node.id] != null" class="latency-badge" :class="latencyClass(latencies[node.id])">{{ latencies[node.id] }} ms</span>
            </div>
            <div class="node-sub">
              {{ showAddr[node.id] ? `${node.server}:${node.port || 443} · ${node.path}` : '点击连接 · 长按编辑' }}
            </div>
          </div>
          <span v-if="status?.connectedId === node.id" class="check-icon">✓</span>
          <div class="node-actions">
            <button class="icon-btn-sm" @click.stop="toggleAddr(node.id)" :title="showAddr[node.id] ? '隐藏地址' : '显示地址'">{{ showAddr[node.id] ? '🙈' : '👁' }}</button>
            <button class="icon-btn-sm" @click.stop="doTestLatency(node)" title="测速">⚡</button>
            <button class="icon-btn-sm" @click.stop="openEdit(node)" title="编辑">✎</button>
          </div>
        </div>

        <!-- Log card -->
        <div class="log-card">
          <div class="log-header">
            <span class="log-title">连接日志</span>
            <button class="icon-btn-sm" @click="refreshLogs" title="刷新">⟳</button>
            <button class="icon-btn-sm" @click="doClearLogs" title="清空">🗑</button>
          </div>
          <div ref="logContainer" class="log-body">
            <span v-if="logs.length === 0" class="log-empty">等待操作…</span>
            <span v-else class="log-text">{{ logs.join('\n') }}</span>
          </div>
        </div>
      </div>
    </template>

    <!-- Settings tab -->
    <template v-if="activeTab === 'settings'">
      <div class="content-scroll">
        <div class="settings-panel" v-if="settings">
          <div class="setting-group">
            <div class="setting-title">代理</div>
            <div class="setting-row">
              <label>SOCKS5 监听地址</label>
              <input v-model="settings.listenAddr" class="setting-input" />
            </div>
          </div>
          <div class="setting-group">
            <div class="setting-title">流量模式</div>
            <div class="setting-row">
              <label>TUN 全局 VPN (Wintun)</label>
              <input type="checkbox" v-model="settings.tunMode" />
            </div>
            <div class="setting-hint">需要管理员权限。关闭时使用系统代理。</div>
            <div class="setting-row">
              <label>自动设置系统代理</label>
              <input type="checkbox" v-model="settings.autoSysProxy" :disabled="settings.tunMode" />
            </div>
          </div>
          <div class="setting-group">
            <div class="setting-title">启动</div>
            <div class="setting-row">
              <label>开机自动连接</label>
              <input type="checkbox" v-model="settings.autoConnect" />
            </div>
          </div>
          <div class="setting-group">
            <div class="setting-title">系统代理状态</div>
            <div class="setting-row">
              <label>当前状态</label>
              <button class="btn" :class="status?.sysProxyOn ? 'btn-primary' : ''" @click="toggleSysProxy">
                {{ status?.sysProxyOn ? '已开启' : '已关闭' }}
              </button>
            </div>
          </div>
          <button class="btn btn-primary save-btn" @click="saveSettings">保存设置</button>
        </div>
      </div>
    </template>

    <!-- About tab -->
    <template v-if="activeTab === 'about'">
      <div class="content-scroll">
        <div class="about-hero">
          <div class="about-logo">365</div>
          <div class="about-name">Open365VPN</div>
          <div class="about-version">version 1.0.0</div>
          <div class="about-desc">X365 协议 VPN 客户端<br>Reality TLS · Wintun tun2socks · SOCKS5</div>
        </div>
        <div class="about-section">
          <div class="about-section-title">开源信息</div>
          <div class="license-item">
            <span class="license-name">tun2socks (gVisor)</span>
            <span class="license-type">Apache License 2.0</span>
          </div>
          <div class="license-item">
            <span class="license-name">Wintun</span>
            <span class="license-type">MIT License</span>
          </div>
          <div class="license-item">
            <span class="license-name">uTLS</span>
            <span class="license-type">BSD 3-Clause</span>
          </div>
          <div class="license-item">
            <span class="license-name">Wails v2</span>
            <span class="license-type">MIT License</span>
          </div>
        </div>
      </div>
    </template>

    <!-- Error bar -->
    <div v-if="errorMsg" class="error-bar" @click="errorMsg = ''">{{ errorMsg }}</div>

    <!-- Edit modal -->
    <div v-if="editTarget" class="modal-overlay" @click.self="editTarget = null">
      <div class="modal-card">
        <div class="modal-header">
          <span class="modal-flag">{{ isoToFlag(editTarget.countryCode) }}</span>
          <span class="modal-title">编辑节点</span>
        </div>
        <input v-model="editName" class="setting-input modal-input" placeholder="名称" />
        <div class="modal-details">
          <div class="detail-row"><span class="detail-key">服务器</span><span class="detail-val">{{ editTarget.server }}:{{ editTarget.port || 443 }}</span></div>
          <div class="detail-row"><span class="detail-key">路径</span><span class="detail-val">{{ editTarget.path }}</span></div>
          <div class="detail-row"><span class="detail-key">SNI</span><span class="detail-val">{{ editTarget.sni }}</span></div>
          <div class="detail-row"><span class="detail-key">UUID</span><span class="detail-val">{{ maskOr(editTarget.uuid, editShowSensitive) }}</span></div>
          <div class="detail-row"><span class="detail-key">公钥</span><span class="detail-val">{{ maskOr(editTarget.pbk, editShowSensitive) }}</span></div>
          <div class="detail-row"><span class="detail-key">短ID</span><span class="detail-val">{{ maskOr(editTarget.sid, editShowSensitive) }}</span></div>
        </div>
        <div class="modal-actions">
          <button class="btn-sm" @click="editShowSensitive = !editShowSensitive">{{ editShowSensitive ? '隐藏密钥' : '显示密钥' }}</button>
        </div>
        <div class="modal-footer">
          <button class="btn btn-primary" @click="saveEdit">保存</button>
          <button class="btn btn-outline-danger" @click="deleteFromEdit">删除</button>
          <button class="btn" @click="editTarget = null">取消</button>
        </div>
      </div>
    </div>
  </div>
</template>