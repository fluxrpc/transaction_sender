class SolanaTransactionSender {
	url;

	epoch

	leaderSchedule = {};
	slotToLeader = {};
	clusterInfo = {};

	constructor(rpcUrl) {
		this.url = rpcUrl;

		this.fetchEpoch()
		this.fetchClusterInfo()
		this.fetchLeaderSchedule();
	}

	async sendTransaction(txData, specificLeaderEndpoint = null) {
		let transport;
		try {
			transport = await this.initTransport(specificLeaderEndpoint);

			const stream = await transport.createUnidirectionalStream();
			const writer = stream.getWriter();

			await writer.write(txData);
			await writer.close();
			writer.releaseLock();
		} catch (err) {
			throw err; // bubble up
		} finally {
			if (transport) {
				await this.closeTransport(transport);
			}
		}
	}

	async initTransport(leader = null) {


		if(!leader) {
			leader = await this.getCurrentLeader()
			if (!leader) {
				throw "unable to find leader"
			}
		} else {
			console.log("Using custom leader", leader)
		}


		const transport = new WebTransport(leader);
		await transport.ready;
		console.log(`Connected to ${this.url}`);
		return transport;
	}

	async closeTransport(transport) {
		try {
			await transport.closed;
		} catch (err) {
			throw err;
		}
	}

	async fetchLeaderSchedule() {
		console.log("fetching leader schedule")
		const res = await this._rpcCall('getLeaderSchedule');
		this.leaderSchedule = res?.result || null;

		Object.keys(this.leaderSchedule).forEach(l => {
			this.leaderSchedule[l].forEach(s => this.slotToLeader[s] = l)
		})

		return this.leaderSchedule;
	}

	async fetchSlot() {
		const res = await this._rpcCall('getSlot');
		return res?.result;
	}

	async fetchEpoch() {
		const res = await this._rpcCall('getEpochInfo');
		const epoch = res?.result;
		this.epoch = epoch
		return epoch;
	}

	async fetchClusterInfo() {
		const res = await this._rpcCall('getClusterNodes');
		const info = res?.result || [];

		info.forEach(c => {
			this.clusterInfo[c.pubkey] = c
		})

		return info;
	}

	async getCurrentLeader() {
		if (!this.leaderSchedule) {
			await this.fetchLeaderSchedule();
		}

		const slot = await this.fetchSlot();

		const relSlot = slot - (this.epoch.epoch * this.epoch.slotsInEpoch)
		const currentLeader = this.slotToLeader[relSlot]
		const leaderInfo = this.clusterInfo[currentLeader]
		if (!leaderInfo) {
			console.warn(`No leader found for slot ${slot}`);
			return null
		}

		return `https://${leaderInfo.tpuQuic}`
	}


	async _rpcCall(method, params = []) {
		try {
			const res = await fetch(this.url, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					jsonrpc: '2.0',
					id: 1,
					method,
					params
				})
			});
			if (!res.ok) {
				throw new Error(`RPC call failed: ${res.status} ${res.statusText}`);
			}
			return await res.json();
		} catch (err) {
			console.error(`RPC error for ${method}:`, err);
			throw err;
		}
	}
}
